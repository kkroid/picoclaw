package prepare

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCompileProtocolClientOnePilotArtifacts(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText:   onePilotProtocolClientFixture(),
		RequirementSource: "test-onepilot-protocol-client",
		TemplateID:        "flutter-open-lite",
		RealBuild:         true,
		Now:               func() time.Time { return time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	var domainModel DomainModel
	if err := json.Unmarshal(bundle.Files[domainModelFileName], &domainModel); err != nil {
		t.Fatalf("unmarshal domain-model.json error = %v", err)
	}
	if domainModel.PersistenceContract == nil || domainModel.PersistenceContract.Mode != "client-settings-shared-preferences" {
		t.Fatalf("PersistenceContract = %#v, want client settings SharedPreferences", domainModel.PersistenceContract)
	}
	if domainModel.ProtocolContract == nil || len(domainModel.ProtocolContract.Endpoints) == 0 {
		t.Fatalf("ProtocolContract = %#v, want endpoints", domainModel.ProtocolContract)
	}
	if domainModel.RealtimeContract == nil || domainModel.RealtimeContract.Transport != "websocket" {
		t.Fatalf("RealtimeContract = %#v, want websocket", domainModel.RealtimeContract)
	}
	if domainModel.RuntimeContract == nil || !containsDependency(domainModel.RuntimeContract.Dependencies, "web_socket_channel") {
		t.Fatalf("RuntimeContract dependencies = %#v, want web_socket_channel", domainModel.RuntimeContract)
	}
	if !runtimeContractPlatformPurposeContains(domainModel.RuntimeContract, "android", "arm64-v8a") || !runtimeContractPlatformPurposeContains(domainModel.RuntimeContract, "ios", "iPhone 13") {
		t.Fatalf("RuntimeContract platform_config = %#v, want Android arm64-v8a and iOS iPhone 13+ targets", domainModel.RuntimeContract.PlatformConfig)
	}
	for _, forbidden := range []string{"通用记录", "local-hive", "默认离线单机运行", "不把远端 API 作为默认前提"} {
		for _, fileName := range []string{domainModelFileName, prdMarkdownFileName, constraintsFileName} {
			if strings.Contains(string(bundle.Files[fileName]), forbidden) {
				t.Fatalf("protocol-client artifact %s contains forbidden generic local marker %q", fileName, forbidden)
			}
		}
	}
	for _, entityID := range []string{"entity-project", "entity-conversation", "entity-message", "entity-message-segment", "entity-file-node", "entity-connection-settings"} {
		if !domainModelHasEntity(domainModel, entityID) {
			t.Fatalf("domain model missing entity %s", entityID)
		}
	}
	for _, endpointID := range []string{"endpoint-system-info", "endpoint-projects-list", "endpoint-conversations-list", "endpoint-send-turn", "endpoint-emergency-stop", "endpoint-files-tree", "endpoint-files-read", "endpoint-files-write"} {
		if !protocolContractHasEndpoint(domainModel.ProtocolContract, endpointID, true, false) {
			t.Fatalf("protocol contract missing required endpoint %s", endpointID)
		}
	}
	for _, endpointID := range []string{"endpoint-project-start", "endpoint-project-stop", "endpoint-project-delete", "endpoint-files-search", "endpoint-system-qrcode"} {
		if !protocolContractHasEndpoint(domainModel.ProtocolContract, endpointID, false, true) {
			t.Fatalf("protocol contract missing excluded endpoint %s", endpointID)
		}
	}

	var slotMap TemplateSlotMap
	if err := json.Unmarshal(bundle.Files[templateSlotMapFileName], &slotMap); err != nil {
		t.Fatalf("unmarshal template-slot-map.json error = %v", err)
	}
	if !templateSlotMapHasBinding(slotMap, "protocol-services") || !templateSlotMapHasBinding(slotMap, "protocol-surfaces") || !templateSlotMapHasBinding(slotMap, "protocol-tests") {
		t.Fatalf("protocol slot map missing service/surface/test bindings: %#v", slotMap.Slots)
	}

	var allocation TaskAllocation
	if err := json.Unmarshal(bundle.Files[taskAllocationFileName], &allocation); err != nil {
		t.Fatalf("unmarshal task-allocation.json error = %v", err)
	}
	if !taskAllocationTargets(allocation, "lib/services/api_client.dart") || !taskAllocationTargets(allocation, "lib/screens/conversations/detail_page.dart") || !taskAllocationTargets(allocation, "test/widget_test.dart") || !taskAllocationTargets(allocation, "android/app/build.gradle.kts") {
		t.Fatalf("protocol task allocation missing required target paths: %#v", allocation.Units)
	}

	for _, checkID := range []string{"check-protocol-endpoints", "check-protocol-negative-capabilities", "check-protocol-runtime-bootstrap", "check-protocol-platform-targets", "check-protocol-no-generic-shell"} {
		if !builderInputHasAcceptanceCheck(bundle, checkID) {
			t.Fatalf("builder input missing acceptance check %s", checkID)
		}
	}
	if !containsString(bundle.BuilderInput.AllowedPaths, "android/app/src/main/AndroidManifest.xml") {
		t.Fatalf("AllowedPaths = %v, want Android manifest writable for protocol network config", bundle.BuilderInput.AllowedPaths)
	}
}

func containsDependency(dependencies []DependencyContract, name string) bool {
	for _, dependency := range dependencies {
		if dependency.Name == name {
			return true
		}
	}
	return false
}

func runtimeContractPlatformPurposeContains(contract *RuntimeDependencyContract, platform, marker string) bool {
	if contract == nil {
		return false
	}
	for _, platformConfig := range contract.PlatformConfig {
		if platformConfig.Platform == platform && strings.Contains(platformConfig.Purpose, marker) {
			return true
		}
	}
	return false
}

func domainModelHasEntity(domainModel DomainModel, entityID string) bool {
	for _, entity := range domainModel.Entities {
		if entity.EntityID == entityID {
			return true
		}
	}
	return false
}

func protocolContractHasEndpoint(contract *ProtocolContract, endpointID string, required, excluded bool) bool {
	if contract == nil {
		return false
	}
	for _, endpoint := range contract.Endpoints {
		if endpoint.EndpointID == endpointID && endpoint.Required == required && endpoint.Excluded == excluded {
			return true
		}
	}
	return false
}

func templateSlotMapHasBinding(slotMap TemplateSlotMap, bindingID string) bool {
	for _, slot := range slotMap.Slots {
		if slot.BindingID == bindingID {
			return true
		}
	}
	return false
}

func taskAllocationTargets(allocation TaskAllocation, targetPath string) bool {
	for _, unit := range allocation.Units {
		if containsString(unit.TargetPaths, targetPath) {
			return true
		}
	}
	return false
}

func builderInputHasAcceptanceCheck(bundle Bundle, checkID string) bool {
	for _, check := range bundle.BuilderInput.AcceptanceChecks {
		if check.CheckID == checkID {
			return true
		}
	}
	return false
}

func onePilotProtocolClientFixture() string {
	return strings.Join([]string{
		"做一个 OnePilot 手机 App MVP，用 Flutter 实现，通过局域网连接 PC 端 OnePilot Server。",
		"Android 只支持 arm64-v8a，iOS 目标为 iPhone 13 及以上 64 位设备。",
		"基础地址：http://{pc_ip}:{port}/api/v1。",
		"需要使用 GET /system/info、GET /projects、GET /projects/{id}、GET /conversations、POST /conversations、GET /conversations/{id}、POST /conversations/{id}/turns、POST /conversations/{id}/emergency-stop、GET /files/tree、GET /files/read、PUT /files/write。",
		"WebSocket 地址 ws://{pc_ip}:{port}/ws?project_id=xxx，需要 subscribe、unsubscribe、delta、turn_completed、conversation_update、instance_status、replay_truncated。",
		"使用 http、web_socket_channel、flutter_markdown、shared_preferences、provider。",
		"不做 POST /projects、POST /projects/{id}/start、POST /projects/{id}/stop、DELETE /projects/{id}、GET /files/search、GET /system/qrcode。",
	}, "\n")
}
