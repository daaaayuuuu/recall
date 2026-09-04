package generation

import (
	"database/sql"
	"testing"
	"time"
)

func TestRunDTOSeparatesCreatorAndAdminDiagnostics(t *testing.T) {
	now := time.Date(2026, 8, 13, 6, 0, 0, 0, time.UTC)
	run := Run{
		ID: "run", GameID: "game", GameVersionID: "version", AttemptNumber: 2,
		Status: "failed", Stage: "completed", Progress: 100, ErrorCode: sql.NullString{String: "PROVIDER_UNAVAILABLE", Valid: true},
		AdminMessage: sql.NullString{String: "sanitized admin diagnosis", Valid: true}, SanitizedDetails: []byte(`{"errorType":"mock"}`),
		Retryable: true, TraceID: "trace", CreatedAt: now, UpdatedAt: now,
	}
	creator := runDTO(run, false)
	if _, exists := creator["adminMessage"]; exists {
		t.Fatal("creator DTO must not contain admin diagnostics")
	}
	if creator["errorMessage"] != "创建服务暂时不可用，可以稍后重试" {
		t.Fatalf("unexpected creator error message: %#v", creator["errorMessage"])
	}
	admin := runDTO(run, true)
	if admin["adminMessage"] != "sanitized admin diagnosis" || admin["traceId"] != "trace" {
		t.Fatalf("missing admin diagnostics: %#v", admin)
	}
}

func TestRunDTOExplainsOversizedImageToCreator(t *testing.T) {
	run := Run{
		ErrorCode:        sql.NullString{String: "INPUT_VALIDATION_FAILED", Valid: true},
		SanitizedDetails: []byte(`{"errorType":"image_input_limit"}`),
	}
	creator := runDTO(run, false)
	if creator["errorMessage"] != "照片处理后的文件超过生成上限，请压缩图片或缩小尺寸后重新上传" {
		t.Fatalf("unexpected creator error message: %#v", creator["errorMessage"])
	}
}

func TestValidRunStatus(t *testing.T) {
	for _, status := range []string{"queued", "running", "succeeded", "failed", "cancelled"} {
		if !validRunStatus(status) {
			t.Fatalf("expected %q to be valid", status)
		}
	}
	if validRunStatus("deleting") {
		t.Fatal("unexpected generation status accepted")
	}
}
