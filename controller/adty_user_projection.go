package controller

import "github.com/QuantumNous/new-api/model"

// adtyHideUpstreamModel 清掉任务属性里的上游模型名，普通用户只看到自己请求的模型（任务 7.1）。
func adtyHideUpstreamModel(properties model.Properties) model.Properties {
	properties.UpstreamModelName = ""
	return properties
}
