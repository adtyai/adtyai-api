package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAdtyPaymentScenes(c *gin.Context) {
	config, err := service.GetAdtyPaymentScenes()
	if err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}

func PutAdtyPaymentScenes(c *gin.Context) {
	var input struct {
		ExpectedVersion int64                    `json:"expected_version"`
		Scenes          []model.AdtyPaymentScene `json:"scenes"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	if common.DecodeJson(c.Request.Body, &input) != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	audit := prepareManageAudit(c, 0, "adty.payment_scenes.update", nil)
	config, err := service.SaveAdtyPaymentScenes(c.GetInt("id"), input.ExpectedVersion, input.Scenes, audit)
	if errors.Is(err, model.ErrAdtyPaymentSceneConflict) {
		common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("The payment scene configuration has changed. Reload and try again"))
		return
	}
	if err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}
