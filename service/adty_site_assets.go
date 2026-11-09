package service

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// 站长上传的 Logo（任务 11.3）：只收 PNG、JPEG、WebP，不收 SVG。最大 200 KB，存在 ./uploads/site-assets（工作目录是 /data，
// 跟着 /data 一起备份）。地址是 /uploads/site-assets/<随机名>，写进 Brand.logo_url 由站长在 PUT /self 保存。

// AdtySiteLogoMaxBytes 是 Logo 的大小上限。
const AdtySiteLogoMaxBytes = 200 << 10

var (
	ErrAdtySiteLogoTooLarge = common.NewMessage("The logo must be at most 200 KB")
	ErrAdtySiteLogoType     = common.NewMessage("The logo must be a PNG, JPEG, or WebP image")
	ErrAdtySiteLogoUnsafe   = common.NewMessage("The SVG contains scripts, links or other active content")
)

// adtySiteAssetDir 是存放目录，测试可以换。
var adtySiteAssetDir = func() string { return filepath.Join("uploads", "site-assets") }

// adtySiteAssetName 是存储文件名的格式：site-<站点 id>-<uuid>.<扩展名>。
var adtySiteAssetName = regexp.MustCompile(`^site-[0-9]+-[0-9a-f]{32}\.(png|jpg|webp|svg)$`)

// SVG 允许的元素：只有静态图形、渐变、裁剪和文字。没有 script、style、foreignObject、use、image、a、动画。
var adtySvgElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true, "path": true, "rect": true, "circle": true,
	"ellipse": true, "line": true, "polyline": true, "polygon": true, "text": true, "tspan": true,
	"lineargradient": true, "radialgradient": true, "stop": true, "clippath": true, "mask": true, "pattern": true,
	"symbol": true, "metadata": true,
}

// SVG 允许的属性（小写，不含命名空间前缀）。没有 on*、href、style 以外的 URL 属性。
var adtySvgAttributes = map[string]bool{
	"id": true, "class": true, "xmlns": true, "version": true, "width": true, "height": true, "viewbox": true,
	"preserveaspectratio": true, "x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true, "cx": true,
	"cy": true, "r": true, "rx": true, "ry": true, "d": true, "points": true, "transform": true, "fill": true,
	"fill-opacity": true, "fill-rule": true, "clip-rule": true, "stroke": true, "stroke-width": true,
	"stroke-opacity": true, "stroke-linecap": true, "stroke-linejoin": true, "stroke-miterlimit": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "opacity": true, "offset": true, "stop-color": true,
	"stop-opacity": true, "gradientunits": true, "gradienttransform": true, "spreadmethod": true, "fx": true,
	"fy": true, "clip-path": true, "clippathunits": true, "mask": true, "maskunits": true, "patternunits": true,
	"patterntransform": true, "font-family": true, "font-size": true, "font-weight": true, "font-style": true,
	"text-anchor": true, "dominant-baseline": true, "letter-spacing": true, "dx": true, "dy": true,
	"xml:space": true, "space": true, "lang": true, "data-name": true,
}

// adtySvgURL 匹配属性值里引用外部资源的写法；只允许 url(#本文档内的 id)。
var adtySvgURL = regexp.MustCompile(`(?i)url\(\s*['"]?\s*[^#\s'")]`)

// ValidateAdtySiteSVG 检查一个 SVG 只含白名单里的静态内容。
func ValidateAdtySiteSVG(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	// 不展开外部实体，不读 DTD 外的东西。
	decoder.Entity = map[string]string{}
	sawRoot := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ErrAdtySiteLogoType
		}
		switch tok := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(tok.Name.Local)
			if !sawRoot {
				if name != "svg" {
					return ErrAdtySiteLogoType
				}
				sawRoot = true
			}
			if !adtySvgElements[name] {
				return ErrAdtySiteLogoUnsafe
			}
			for _, attr := range tok.Attr {
				attrName := strings.ToLower(attr.Name.Local)
				if attr.Name.Space == "xmlns" || (attr.Name.Space == "" && attrName == "xmlns") {
					continue // 命名空间声明
				}
				if !adtySvgAttributes[attrName] || adtySvgURL.MatchString(attr.Value) || strings.Contains(strings.ToLower(attr.Value), "javascript:") {
					return ErrAdtySiteLogoUnsafe
				}
			}
		case xml.Directive:
			// <!DOCTYPE ...> 和 <!ENTITY ...> 可以引入外部实体，一律不收。
			return ErrAdtySiteLogoUnsafe
		case xml.ProcInst:
			if strings.ToLower(tok.Target) != "xml" {
				return ErrAdtySiteLogoUnsafe
			}
		}
	}
	if !sawRoot {
		return ErrAdtySiteLogoType
	}
	return nil
}

// adtySiteLogoKind 判断文件类型：只收 PNG、JPEG 和 WebP。不收 SVG。
func adtySiteLogoKind(data []byte) (ext, contentType string, err error) {
	switch http.DetectContentType(data) {
	case "image/png":
		return ".png", "image/png", nil
	case "image/jpeg":
		return ".jpg", "image/jpeg", nil
	}
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return ".webp", "image/webp", nil
	}
	return "", "", ErrAdtySiteLogoType
}

// SaveAdtySiteLogo 存一个 Logo，返回地址（/uploads/site-assets/<名字>）。不改站点：站长在 PUT /self 里保存地址。
func SaveAdtySiteLogo(siteId int, data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrAdtySiteLogoType
	}
	if len(data) > AdtySiteLogoMaxBytes {
		return "", ErrAdtySiteLogoTooLarge
	}
	ext, _, err := adtySiteLogoKind(data)
	if err != nil {
		return "", err
	}
	dir := adtySiteAssetDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	name := "site-" + strconv.Itoa(siteId) + "-" + strings.ReplaceAll(common.GetUUID(), "-", "") + ext
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o640); err != nil {
		return "", err
	}
	return AdtySiteAssetPrefix + name, nil
}

// OpenAdtySiteAsset 按文件名找到 Logo，返回路径和 Content-Type；名字不合格式或文件不在时返回 os.ErrNotExist。
func OpenAdtySiteAsset(name string) (string, string, error) {
	if !adtySiteAssetName.MatchString(name) {
		return "", "", os.ErrNotExist
	}
	path := filepath.Join(adtySiteAssetDir(), name)
	if _, err := os.Stat(path); err != nil {
		return "", "", os.ErrNotExist
	}
	contentType := "image/png"
	switch filepath.Ext(name) {
	case ".jpg":
		contentType = "image/jpeg"
	case ".svg":
		contentType = "image/svg+xml"
	case ".webp":
		contentType = "image/webp"
	}
	return path, contentType, nil
}

// SetAdtySiteAssetDirForTest 换 Logo 的存放目录，返回恢复函数（测试用）。
func SetAdtySiteAssetDirForTest(dir string) func() {
	old := adtySiteAssetDir
	adtySiteAssetDir = func() string { return dir }
	return func() { adtySiteAssetDir = old }
}
