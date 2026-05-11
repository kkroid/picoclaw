package emitter

import (
	"strings"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

// BrandEmitResult 表示 BrandEmitter 的输出。
type BrandEmitResult struct {
	// StringsXML 是 android/app/src/main/res/values/strings.xml 的完整内容。
	StringsXML     string
	StringsXMLPath string
}

// EmitBrand 从 DomainModel 确定性生成 Android branding 文件。
// 返回 (result, true) 表示成功；(zero, false) 表示输入不足无法 emit。
func EmitBrand(dm appprepare.DomainModel) (BrandEmitResult, bool) {
	appTitle := strings.TrimSpace(dm.DomainCopy.Title)
	if appTitle == "" {
		return BrandEmitResult{}, false
	}
	return BrandEmitResult{
		StringsXML:     renderStringsXML(appTitle),
		StringsXMLPath: "android/app/src/main/res/values/strings.xml",
	}, true
}

// renderStringsXML 生成 Android strings.xml 完整内容。
func renderStringsXML(appTitle string) string {
	return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n    <string name=\"app_name\">" + escapeXML(appTitle) + "</string>\n</resources>"
}

// escapeXML 对 XML 文本内容做最小必要转义。
func escapeXML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}
