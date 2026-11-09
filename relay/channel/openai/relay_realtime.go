package openai

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const realtimeWSWriteTimeout = 30 * time.Second
const realtimeWSCloseDrainTimeout = 2 * time.Second

type realtimeReaderResult struct {
	fromClient bool
	err        error
}

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	readLimit := int64(maxMB) << 20
	clientConn.SetReadLimit(readLimit)
	targetConn.SetReadLimit(readLimit)
	requestDone := c.Request.Context().Done()

	readerResults := make(chan realtimeReaderResult, 2)
	clientWriteFailed := make(chan error, 1)
	var readers sync.WaitGroup
	readers.Add(2)
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			_ = clientConn.Close()
			_ = targetConn.Close()
			readers.Wait()
		})
	}
	defer stop()

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}
	var usageMu sync.Mutex

	gopool.Go(func() {
		var readerErr error
		defer readers.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				readerErr = fmt.Errorf("panic in client reader: %v", recovered)
			}
			readerResults <- realtimeReaderResult{fromClient: true, err: readerErr}
		}()
		for {
			select {
			case <-requestDone:
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						readerErr = fmt.Errorf("error reading from client: %v", err)
					}
					return
				}

				realtimeEvent := &dto.RealtimeEvent{}
				if err := common.Unmarshal(message, realtimeEvent); err != nil {
					readerErr = fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				usageMu.Lock()
				if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate && realtimeEvent.Session != nil && realtimeEvent.Session.Tools != nil {
					info.RealtimeTools = realtimeEvent.Session.Tools
				}
				textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
				if err == nil {
					localUsage.TotalTokens += textToken + audioToken
					localUsage.InputTokens += textToken + audioToken
					localUsage.InputTokenDetails.TextTokens += textToken
					localUsage.InputTokenDetails.AudioTokens += audioToken
				}
				usageMu.Unlock()
				if err != nil {
					readerErr = fmt.Errorf("error counting text token: %v", err)
					return
				}
				logger.LogInfo(c, common.LogText("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))

				if err := writeRealtimeMessage(c, targetConn, message); err != nil {
					readerErr = fmt.Errorf("error writing to target: %v", err)
					return
				}
			}
		}
	})

	gopool.Go(func() {
		var readerErr error
		clientWritable := true
		defer readers.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				readerErr = fmt.Errorf("panic in target reader: %v", recovered)
			}
			readerResults <- realtimeReaderResult{err: readerErr}
		}()
		for {
			select {
			case <-requestDone:
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						readerErr = fmt.Errorf("error reading from target: %v", err)
					}
					return
				}
				info.SetFirstResponseTime()
				realtimeEvent := &dto.RealtimeEvent{}
				if err := common.Unmarshal(message, realtimeEvent); err != nil {
					readerErr = fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				usageMu.Lock()
				if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
					var realtimeUsage *dto.RealtimeUsage
					if realtimeEvent.Response != nil {
						realtimeUsage = realtimeEvent.Response.Usage
					}
					if realtimeUsage != nil {
						usage.TotalTokens += realtimeUsage.TotalTokens
						usage.InputTokens += realtimeUsage.InputTokens
						usage.OutputTokens += realtimeUsage.OutputTokens
						usage.InputTokenDetails.AudioTokens += realtimeUsage.InputTokenDetails.AudioTokens
						usage.InputTokenDetails.CachedTokens += realtimeUsage.InputTokenDetails.CachedTokens
						usage.InputTokenDetails.TextTokens += realtimeUsage.InputTokenDetails.TextTokens
						usage.OutputTokenDetails.AudioTokens += realtimeUsage.OutputTokenDetails.AudioTokens
						usage.OutputTokenDetails.TextTokens += realtimeUsage.OutputTokenDetails.TextTokens
						responseUsage := usage
						usage = &dto.RealtimeUsage{}
						localUsage = &dto.RealtimeUsage{}
						if err := preConsumeUsage(c, info, responseUsage, sumUsage); err != nil {
							usageMu.Unlock()
							readerErr = fmt.Errorf("error consume usage: %v", err)
							return
						}
					} else {
						textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
						if err != nil {
							usageMu.Unlock()
							readerErr = fmt.Errorf("error counting text token: %v", err)
							return
						}
						logger.LogInfo(c, common.LogText("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						localUsage.TotalTokens += textToken + audioToken
						info.IsFirstRequest = false
						localUsage.InputTokens += textToken + audioToken
						localUsage.InputTokenDetails.TextTokens += textToken
						localUsage.InputTokenDetails.AudioTokens += audioToken
						responseUsage := localUsage
						localUsage = &dto.RealtimeUsage{}
						if err := preConsumeUsage(c, info, responseUsage, sumUsage); err != nil {
							usageMu.Unlock()
							readerErr = fmt.Errorf("error consume usage: %v", err)
							return
						}
					}
					logger.LogInfo(c, common.LogText("realtime streaming sumUsage: %v", sumUsage))
					logger.LogInfo(c, common.LogText("realtime streaming localUsage: %v", localUsage))
				} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
					if realtimeEvent.Session != nil {
						info.InputAudioFormat = common.GetStringIfEmpty(realtimeEvent.Session.InputAudioFormat, info.InputAudioFormat)
						info.OutputAudioFormat = common.GetStringIfEmpty(realtimeEvent.Session.OutputAudioFormat, info.OutputAudioFormat)
					}
				} else {
					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err != nil {
						usageMu.Unlock()
						readerErr = fmt.Errorf("error counting text token: %v", err)
						return
					}
					logger.LogInfo(c, common.LogText("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
					localUsage.TotalTokens += textToken + audioToken
					localUsage.OutputTokens += textToken + audioToken
					localUsage.OutputTokenDetails.TextTokens += textToken
					localUsage.OutputTokenDetails.AudioTokens += audioToken
				}
				usageMu.Unlock()

				if clientWritable {
					if err := writeRealtimeMessage(c, clientConn, message); err != nil {
						clientWritable = false
						clientWriteFailed <- fmt.Errorf("error writing to client: %v", err)
					}
				}
			}
		}
	})

	var firstResult *realtimeReaderResult
	select {
	case result := <-readerResults:
		firstResult = &result
	case err := <-clientWriteFailed:
		firstResult = &realtimeReaderResult{fromClient: true, err: err}
	case <-requestDone:
	}
	if firstResult == nil {
		writeRealtimeClose(clientConn)
		writeRealtimeClose(targetConn)
	} else {
		if firstResult.err != nil {
			logger.LogError(c, common.LogText("realtime error: %s", firstResult.err.Error()))
		}
		if firstResult.fromClient {
			writeRealtimeClose(targetConn)
		} else {
			writeRealtimeClose(clientConn)
		}
		timer := time.NewTimer(realtimeWSCloseDrainTimeout)
	drain:
		for {
			select {
			case result := <-readerResults:
				if result.err != nil {
					logger.LogError(c, common.LogText("realtime error: %s", result.err.Error()))
				}
				if result.fromClient != firstResult.fromClient {
					break drain
				}
			case <-timer.C:
				break drain
			case <-requestDone:
				break drain
			}
		}
		timer.Stop()
	}
	stop()

	if usage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, usage, sumUsage)
	}
	if localUsage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, localUsage, sumUsage)
	}

	return nil, sumUsage
}

func writeRealtimeMessage(c *gin.Context, conn *websocket.Conn, body []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(realtimeWSWriteTimeout)); err != nil {
		return err
	}
	return helper.WssString(c, conn, string(body))
}

func writeRealtimeClose(conn *websocket.Conn) {
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens
	return service.PreWssConsumeQuota(ctx, info, usage)
}
