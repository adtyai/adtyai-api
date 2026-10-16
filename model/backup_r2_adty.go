package model

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// R2BackupStore Cloudflare R2（S3 兼容）最小实现：PUT / HEAD / ListObjectsV2 / DELETE，SigV4 签名。
type R2BackupStore struct {
	Endpoint string // https://<account>.r2.cloudflarestorage.com
	Bucket   string
	Prefix   string // 可选：桶内根前缀，例如 adtyai/
	creds    aws.Credentials
	client   *http.Client
	signer   *v4.Signer
}

// NewR2BackupStoreFromEnv 读取 ADTY_BACKUP_R2_{ENDPOINT,BUCKET,ACCESS_KEY_ID,SECRET_ACCESS_KEY,ROOT_PREFIX}。
func NewR2BackupStoreFromEnv() (*R2BackupStore, error) {
	ep := strings.TrimRight(backupEnv("ADTY_BACKUP_R2_ENDPOINT"), "/")
	bucket := backupEnv("ADTY_BACKUP_R2_BUCKET")
	ak := backupEnv("ADTY_BACKUP_R2_ACCESS_KEY_ID")
	sk := strings.TrimSpace(os.Getenv("ADTY_BACKUP_R2_SECRET_ACCESS_KEY"))
	if ep == "" || bucket == "" || ak == "" || sk == "" {
		return nil, errors.New("r2 not configured")
	}
	return &R2BackupStore{
		Endpoint: ep,
		Bucket:   bucket,
		Prefix:   backupEnv("ADTY_BACKUP_R2_ROOT_PREFIX"),
		creds:    aws.Credentials{AccessKeyID: ak, SecretAccessKey: sk},
		client:   &http.Client{Timeout: 30 * time.Minute},
		signer:   v4.NewSigner(),
	}, nil
}

func (s *R2BackupStore) objURL(key string) string {
	return s.Endpoint + "/" + s.Bucket + (&url.URL{Path: "/" + s.Prefix + key}).EscapedPath()
}

const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func (s *R2BackupStore) do(ctx context.Context, req *http.Request, payloadHash string) (*http.Response, error) {
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if err := s.signer.SignHTTP(ctx, s.creds, req, payloadHash, "s3", "auto", time.Now().UTC()); err != nil {
		return nil, err
	}
	return s.client.Do(req)
}

// r2Err 只带状态码和 S3 错误 Code，不回带 URL / 签名。
func r2Err(op string, resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	var e struct {
		Code string `xml:"Code"`
	}
	_ = xml.Unmarshal(b, &e)
	return fmt.Errorf("r2 %s http %d %s", op, resp.StatusCode, e.Code)
}

func (s *R2BackupStore) Put(ctx context.Context, key, localPath string, size int64, sha string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.objURL(key), f)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-amz-meta-sha256", sha)
	resp, err := s.do(ctx, req, sha)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return r2Err("put", resp)
	}
	return nil
}

func (s *R2BackupStore) Head(ctx context.Context, key string) (*BackupObject, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.objURL(key), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.do(ctx, req, emptyPayloadHash)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("r2 head http %d", resp.StatusCode)
	}
	n, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	return &BackupObject{Key: key, Size: n, SHA256: resp.Header.Get("x-amz-meta-sha256")}, nil
}

type r2ListResult struct {
	Contents []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

func (s *R2BackupStore) List(ctx context.Context, prefix string) ([]BackupObject, error) {
	var out []BackupObject
	token := ""
	for i := 0; i < 100; i++ {
		q := url.Values{}
		q.Set("list-type", "2")
		q.Set("prefix", s.Prefix+prefix)
		if token != "" {
			q.Set("continuation-token", token)
		}
		u := s.Endpoint + "/" + s.Bucket + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.do(ctx, req, emptyPayloadHash)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 2 {
			e := r2Err("list", resp)
			resp.Body.Close()
			return nil, e
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var lr r2ListResult
		if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&lr); err != nil {
			return nil, err
		}
		for _, c := range lr.Contents {
			out = append(out, BackupObject{Key: strings.TrimPrefix(c.Key, s.Prefix), Size: c.Size})
		}
		if !lr.IsTruncated || lr.NextContinuationToken == "" {
			break
		}
		token = lr.NextContinuationToken
	}
	return out, nil
}

func (s *R2BackupStore) Delete(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.objURL(key), nil)
	if err != nil {
		return err
	}
	resp, err := s.do(ctx, req, emptyPayloadHash)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return r2Err("delete", resp)
	}
	return nil
}

func (s *R2BackupStore) Get(ctx context.Context, key string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.objURL(key), nil)
	if err != nil {
		return err
	}
	resp, err := s.do(ctx, req, emptyPayloadHash)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return r2Err("get", resp)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}
