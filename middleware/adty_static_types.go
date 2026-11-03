package middleware

import "mime"

// Go 自带的类型表没有 .webmanifest，镜像（debian-slim）里也没有 /etc/mime.types，
// 不登记的话 ServeFrontendFiles 会按内容猜成 text/plain，浏览器就不认 manifest。
func init() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}
