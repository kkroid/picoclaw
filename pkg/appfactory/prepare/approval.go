package prepare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func buildPRDApprovalRecord(spec domainSpec, prd PRD, prdMarkdown, requirement string, now time.Time) appruns.ApprovalRecord {
	return appruns.ApprovalRecord{
		SchemaVersion:  defaultSchemaVersion,
		ApprovalID:     "approval-prd-" + spec.PRDID,
		ApprovalType:   appruns.ApprovalTypePRD,
		JobID:          spec.JobID,
		PRDID:          spec.PRDID,
		SubjectVersion: PRDApprovalSubjectVersion(prd, []byte(prdMarkdown), []byte(requirement)),
		Status:         appruns.ApprovalStatusApproved,
		RequestedBy:    approvalSystemActor(),
		Decision: &appruns.ApprovalDecision{
			Decision:  appruns.ApprovalStatusApproved,
			DecidedBy: approvalSystemActor(),
			DecidedAt: now.Format(time.RFC3339),
			Comment:   "系统为当前最小编排链路生成已批准 PRD 快照，用于后续执行接线。",
		},
		Summary:       "当前需求已整理为可执行 PRD 快照。",
		EvidencePaths: []string{prdMarkdownFileName, prdJSONFileName, requirementFileName},
		CreatedAt:     now.Format(time.RFC3339),
	}
}

func PRDCompileSourceVersion(prd PRD) string {
	prdID := strings.TrimSpace(prd.ID)
	if prdID == "" {
		prdID = "unknown-prd"
	}
	version := strings.TrimSpace(prd.Version)
	if version == "" {
		version = "unknown"
	}
	data, err := json.Marshal(prd)
	if err != nil {
		return prdID + "@" + version + "@invalid"
	}
	sum := sha256.Sum256(data)
	return prdID + "@" + version + "@sha256:" + hex.EncodeToString(sum[:8])
}

func PRDApprovalSubjectVersion(prd PRD, prdMarkdown, requirement []byte) string {
	base := PRDCompileSourceVersion(prd)
	payload, err := json.Marshal(struct {
		PRDMarkdown string `json:"prd_markdown"`
		Requirement string `json:"requirement"`
	}{
		PRDMarkdown: string(prdMarkdown),
		Requirement: string(requirement),
	})
	if err != nil {
		payload = append(append(append([]byte(nil), prdMarkdown...), '\n'), requirement...)
	}
	sum := sha256.Sum256(payload)
	return base + "@sha256:" + hex.EncodeToString(sum[:8])
}

func buildTemplateApprovalRecord(spec domainSpec, prd PRD, fitReport string, now time.Time) appruns.ApprovalRecord {
	subjectVersion := TemplateApprovalSubjectVersion(spec.TemplateID, spec.TemplatePinnedRef, []byte(fitReport))
	return appruns.ApprovalRecord{
		SchemaVersion:  defaultSchemaVersion,
		ApprovalID:     "approval-template-" + spec.TemplateID,
		ApprovalType:   appruns.ApprovalTypeTemplate,
		JobID:          spec.JobID,
		PRDID:          spec.PRDID,
		SubjectVersion: subjectVersion,
		Status:         appruns.ApprovalStatusApproved,
		RequestedBy:    approvalSystemActor(),
		Decision: &appruns.ApprovalDecision{
			Decision:  appruns.ApprovalStatusApproved,
			DecidedBy: approvalSystemActor(),
			DecidedAt: now.Format(time.RFC3339),
			Comment:   "系统为当前最小编排链路生成已批准模板快照，用于 builder-input 接线。",
		},
		Summary:       "当前模板选择已冻结到本次执行快照。",
		EvidencePaths: []string{fitReportFileName, prdJSONFileName},
		CreatedAt:     now.Format(time.RFC3339),
	}
}

func TemplateCompileSourceVersion(templateID, pinnedRef string) string {
	trimmedTemplateID := strings.TrimSpace(templateID)
	if trimmedTemplateID == "" {
		trimmedTemplateID = "unknown-template"
	}
	trimmedPinnedRef := strings.TrimSpace(pinnedRef)
	if trimmedPinnedRef == "" {
		trimmedPinnedRef = "unknown"
	}
	base := "selected-template@" + trimmedTemplateID + "@" + trimmedPinnedRef
	if digest := templateSlotRegistryDigest(trimmedTemplateID); digest != "" {
		return fmt.Sprintf("%s@sha256:%s", base, digest)
	}
	return base
}

func TemplateApprovalSubjectVersion(templateID, pinnedRef string, fitReport []byte) string {
	base := TemplateCompileSourceVersion(templateID, pinnedRef)
	sum := sha256.Sum256(fitReport)
	return base + "@sha256:" + hex.EncodeToString(sum[:8])
}

func approvalSystemActor() appruns.ApprovalActor {
	return appruns.ApprovalActor{ActorType: "system", ActorID: "appfactory-prepare"}
}

func templateSlotRegistryDigest(templateID string) string {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return ""
	}
	var payload any
	switch templateID {
	case "flutter-finance-lite":
		payload = flutterFinanceLiteTemplateSlots()
	case "flutter-open-lite":
		payload = flutterOpenLiteTemplateSlots()
	default:
		return ""
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}
