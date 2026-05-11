package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

func TestEmitBrandBasic(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重记录 App"},
	}
	result, ok := EmitBrand(dm)
	if !ok {
		t.Fatal("EmitBrand returned false for valid DomainModel")
	}
	if result.StringsXMLPath != "android/app/src/main/res/values/strings.xml" {
		t.Fatalf("StringsXMLPath = %q, want android/app/src/main/res/values/strings.xml", result.StringsXMLPath)
	}
	want := `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">体重记录 App</string>
</resources>`
	if result.StringsXML != want {
		t.Fatalf("StringsXML =\n%s\nwant:\n%s", result.StringsXML, want)
	}
}

func TestEmitBrandProjectTaskTag(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目任务协同 App"},
	}
	result, ok := EmitBrand(dm)
	if !ok {
		t.Fatal("EmitBrand returned false")
	}
	if !strings.Contains(result.StringsXML, "项目任务协同 App") {
		t.Fatal("StringsXML missing domain title")
	}
}

func TestEmitBrandInventory(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存盘点工作台"},
	}
	result, ok := EmitBrand(dm)
	if !ok {
		t.Fatal("EmitBrand returned false")
	}
	if !strings.Contains(result.StringsXML, "库存盘点工作台") {
		t.Fatal("StringsXML missing domain title")
	}
}

func TestEmitBrandEmptyTitleReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: ""},
	}
	_, ok := EmitBrand(dm)
	if ok {
		t.Fatal("EmitBrand should return false for empty title")
	}
}

func TestEmitBrandWhitespaceTitleReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "   "},
	}
	_, ok := EmitBrand(dm)
	if ok {
		t.Fatal("EmitBrand should return false for whitespace-only title")
	}
}

func TestEmitBrandXMLEscaping(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: `Tom & Jerry's "App" <1>`},
	}
	result, ok := EmitBrand(dm)
	if !ok {
		t.Fatal("EmitBrand returned false")
	}
	if !strings.Contains(result.StringsXML, "Tom &amp; Jerry&apos;s &quot;App&quot; &lt;1&gt;") {
		t.Fatalf("StringsXML XML escaping incorrect:\n%s", result.StringsXML)
	}
}

func TestEmitBrandMatchesSeedStructure(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "Open Lite Seed"},
	}
	result, ok := EmitBrand(dm)
	if !ok {
		t.Fatal("EmitBrand returned false")
	}
	// 验证输出与模板种子结构一致
	want := `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">Open Lite Seed</string>
</resources>`
	if result.StringsXML != want {
		t.Fatalf("StringsXML does not match seed structure:\ngot:\n%s\nwant:\n%s", result.StringsXML, want)
	}
}
