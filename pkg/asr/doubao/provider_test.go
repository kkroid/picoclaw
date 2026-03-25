// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package doubao

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"
)

func TestProviderAudioFormat_DefaultsToOggOpus(t *testing.T) {
	p := &provider{}
	got := p.AudioFormat()
	if got.Format != "ogg" {
		t.Fatalf("format = %q, want ogg", got.Format)
	}
	if got.Codec != "opus" {
		t.Fatalf("codec = %q, want opus", got.Codec)
	}
	if got.SampleRate != 16000 {
		t.Fatalf("sample rate = %d, want 16000", got.SampleRate)
	}
	if got.Channels != 1 {
		t.Fatalf("channels = %d, want 1", got.Channels)
	}
}

func TestNewProvider_RequiresAppKeyAndAccessKey(t *testing.T) {
	if _, err := newProvider(map[string]any{"access_key": "access-key"}); err == nil {
		t.Fatal("expected app_key required error")
	}
	if _, err := newProvider(map[string]any{"app_key": "app-key"}); err == nil {
		t.Fatal("expected access_key required error")
	}
}

func TestNewProvider_RejectsLegacyCredentialFields(t *testing.T) {
	if _, err := newProvider(map[string]any{
		"appid":        "3869699744",
		"access_token": "token-value",
		"cluster":      "bigmodel_transcribe",
	}); err == nil {
		t.Fatal("expected legacy credential fields to be rejected")
	}
}

func TestNewProvider_DefaultWSURLMatchesResource(t *testing.T) {
	seedProvider, err := newProvider(map[string]any{
		"app_key":     "app-key",
		"access_key":  "access-key",
		"resource_id": seedResourceID,
	})
	if err != nil {
		t.Fatalf("newProvider(seed) error = %v", err)
	}
	if seedProvider.wsURL != defaultASRAsyncURL {
		t.Fatalf("seed wsURL = %q, want %q", seedProvider.wsURL, defaultASRAsyncURL)
	}

	legacyProvider, err := newProvider(map[string]any{
		"app_key":     "app-key",
		"access_key":  "access-key",
		"resource_id": defaultResourceID,
	})
	if err != nil {
		t.Fatalf("newProvider(legacy) error = %v", err)
	}
	if legacyProvider.wsURL != defaultASRURL {
		t.Fatalf("legacy wsURL = %q, want %q", legacyProvider.wsURL, defaultASRURL)
	}
}

func TestNewProvider_ExplicitWSURLOverridesResourceDefault(t *testing.T) {
	explicitURL := "wss://openspeech.bytedance.com/api/v3/sauc/custom"
	provider, err := newProvider(map[string]any{
		"app_key":     "3869699744",
		"access_key":  "token-value",
		"resource_id": seedResourceID,
		"ws_url":      explicitURL,
	})
	if err != nil {
		t.Fatalf("newProvider(explicit ws_url) error = %v", err)
	}
	if provider.wsURL != explicitURL {
		t.Fatalf("wsURL = %q, want %q", provider.wsURL, explicitURL)
	}
}

func TestAuthHeaders_UseConfiguredAppIDAndAccessToken(t *testing.T) {
	p := &provider{appKey: "3869699744", accessKey: "token-value", resourceID: seedResourceID}
	headers := p.authHeaders("conn-1")
	if got := headers.Get("X-Api-App-Key"); got != "3869699744" {
		t.Fatalf("X-Api-App-Key = %q, want 3869699744", got)
	}
	if got := headers.Get("X-Api-Access-Key"); got != "token-value" {
		t.Fatalf("X-Api-Access-Key = %q, want token-value", got)
	}
	if got := headers.Get("X-Api-Resource-Id"); got != seedResourceID {
		t.Fatalf("X-Api-Resource-Id = %q, want %s", got, seedResourceID)
	}
	if got := headers.Get("X-Api-Connect-Id"); got != "conn-1" {
		t.Fatalf("X-Api-Connect-Id = %q, want conn-1", got)
	}
	if got := headers.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
}

func TestInitRequest_UsesOggOpusInputWithoutAppAuthPayload(t *testing.T) {
	p := &provider{appKey: "app-key", accessKey: "access-key"}
	req := p.initRequest("req-1")

	if _, ok := req["app"]; ok {
		t.Fatal("did not expect auth payload in init request")
	}

	audio, ok := req["audio"].(map[string]any)
	if !ok {
		t.Fatal("expected audio map")
	}
	if audio["format"] != "ogg" {
		t.Fatalf("audio.format = %v, want ogg", audio["format"])
	}
	if audio["codec"] != "opus" {
		t.Fatalf("audio.codec = %v, want opus", audio["codec"])
	}
	if audio["rate"] != 16000 {
		t.Fatalf("audio.rate = %v, want 16000", audio["rate"])
	}

	request, ok := req["request"].(map[string]any)
	if !ok {
		t.Fatal("expected request map")
	}
	if request["reqid"] != "req-1" {
		t.Fatalf("request.reqid = %v, want req-1", request["reqid"])
	}
	if request["sequence"] != 1 {
		t.Fatalf("request.sequence = %v, want 1", request["sequence"])
	}
	if request["model_name"] != "bigmodel" {
		t.Fatalf("request.model_name = %v, want bigmodel", request["model_name"])
	}
}

// ---- buildFrame ----

func TestBuildFrame_HeaderLayout(t *testing.T) {
	frame := buildFrame(msgTypeFullClientRequest, flagNormal, serialJSON, compressGZP, []byte("payload"))

	if len(frame) < 8 {
		t.Fatalf("frame too short: %d bytes", len(frame))
	}
	// byte[0]: (version=0x01 << 4) | header_size=0x01 = 0x11
	if frame[0] != 0x11 {
		t.Errorf("byte[0] = 0x%02x, want 0x11", frame[0])
	}
	// byte[1]: (msgType << 4) | flags
	wantByte1 := byte((msgTypeFullClientRequest << 4) | flagNormal)
	if frame[1] != wantByte1 {
		t.Errorf("byte[1] = 0x%02x, want 0x%02x", frame[1], wantByte1)
	}
	// byte[2]: (serial << 4) | compress
	wantByte2 := byte((serialJSON << 4) | compressGZP)
	if frame[2] != wantByte2 {
		t.Errorf("byte[2] = 0x%02x, want 0x%02x", frame[2], wantByte2)
	}
	// byte[3]: reserved = 0x00
	if frame[3] != 0x00 {
		t.Errorf("byte[3] = 0x%02x, want 0x00", frame[3])
	}
}

func TestBuildFrame_PayloadLength(t *testing.T) {
	payload := []byte("hello world")
	frame := buildFrame(msgTypeAudioOnly, flagLastFrame, serialJSON, compressGZP, payload)

	// bytes[4:8] = big-endian uint32 of len(payload)
	gotLen := binary.BigEndian.Uint32(frame[4:8])
	if gotLen != uint32(len(payload)) {
		t.Errorf("payload length = %d, want %d", gotLen, len(payload))
	}
	if !bytes.Equal(frame[8:], payload) {
		t.Errorf("payload bytes mismatch")
	}
}

func TestBuildFrame_AudioFlags(t *testing.T) {
	frame := buildFrame(msgTypeAudioOnly, flagLastFrame, serialJSON, compressGZP, []byte{})
	// byte[1] should carry flagLastFrame in lower nibble
	if frame[1]&0x0F != flagLastFrame {
		t.Errorf("flags nibble = 0x%x, want 0x%x", frame[1]&0x0F, flagLastFrame)
	}
}

// ---- buildJSONFrame ----

func TestBuildJSONFrame_GzipPayload(t *testing.T) {
	payload := map[string]any{"key": "value"}
	frame, err := buildJSONFrame(msgTypeFullClientRequest, flagNormal, payload)
	if err != nil {
		t.Fatalf("buildJSONFrame: %v", err)
	}
	if len(frame) < 8 {
		t.Fatalf("frame too short: %d", len(frame))
	}
	// bytes[4:8] = payload length
	compressedLen := binary.BigEndian.Uint32(frame[4:8])
	if int(compressedLen) != len(frame)-8 {
		t.Errorf("compressed length field %d != actual %d", compressedLen, len(frame)-8)
	}
	// Verify gzip decompresses to valid JSON containing "key"
	r, err := gzip.NewReader(bytes.NewReader(frame[8:]))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if out["key"] != "value" {
		t.Errorf("got key=%v, want 'value'", out["key"])
	}
}

// ---- buildAudioFrame ----

func TestBuildAudioFrame_NormalFlag(t *testing.T) {
	pcm := make([]byte, 3200) // 100ms of silence
	frame, err := buildAudioFrame(flagNormal, pcm)
	if err != nil {
		t.Fatalf("buildAudioFrame: %v", err)
	}
	if (frame[1] >> 4) != msgTypeAudioOnly {
		t.Errorf("msgType = 0x%x, want 0x%x", frame[1]>>4, msgTypeAudioOnly)
	}
	if frame[1]&0x0F != flagNormal {
		t.Errorf("flags = 0x%x, want 0x%x", frame[1]&0x0F, flagNormal)
	}
}

func TestBuildAudioFrame_LastFlag(t *testing.T) {
	frame, err := buildAudioFrame(flagLastFrame, []byte{})
	if err != nil {
		t.Fatalf("buildAudioFrame: %v", err)
	}
	if frame[1]&0x0F != flagLastFrame {
		t.Errorf("flags = 0x%x, want 0x%x", frame[1]&0x0F, flagLastFrame)
	}
}

func TestBuildAudioFrame_UsesRawSerializationAndPreservesPayload(t *testing.T) {
	audio := []byte{0x01, 0x02, 0x03, 0x04}
	frame, err := buildAudioFrame(flagNormal, audio)
	if err != nil {
		t.Fatalf("buildAudioFrame: %v", err)
	}
	if frame[2]>>4 != serialNone {
		t.Fatalf("serialization nibble = 0x%x, want 0x%x", frame[2]>>4, serialNone)
	}
	r, err := gzip.NewReader(bytes.NewReader(frame[8:]))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	if !bytes.Equal(raw, audio) {
		t.Fatalf("payload mismatch: got %v want %v", raw, audio)
	}
}

// ---- parseASRResult ----

func buildMockResponse(code int, text string, definite bool) []byte {
	payload, _ := json.Marshal(map[string]any{
		"code": code,
		"result": map[string]any{
			"utterances": []map[string]any{
				{"text": text, "definite": definite},
			},
		},
	})
	// 响应格式：4 字节 header + 8 字节跳过 + JSON
	var buf []byte
	buf = append(buf, 0x11, (0x0B<<4)|0x00, 0x00, 0x00) // header
	buf = append(buf, 0, 0, 0, 0, 0, 0, 0, 0)           // 8 bytes skipped
	buf = append(buf, payload...)
	return buf
}

func TestParseASRResult_DefiniteUtterance(t *testing.T) {
	data := buildMockResponse(1000, "你好世界", true)
	text, definite, done := parseASRResult(data)
	if text != "你好世界" {
		t.Errorf("text = %q, want '你好世界'", text)
	}
	if !definite {
		t.Error("definite = false, want true")
	}
	if !done {
		t.Error("done = false, want true")
	}
}

func TestParseASRResult_IndefiniteUtterance(t *testing.T) {
	data := buildMockResponse(1000, "你好", false)
	text, definite, done := parseASRResult(data)
	if text != "你好" {
		t.Errorf("text = %q, want '你好' (intermediate result)", text)
	}
	if definite {
		t.Error("definite = true, want false for non-definite")
	}
	if done {
		t.Error("done = true, want false for non-definite")
	}
}

func TestParseASRResult_NoSpeechCode(t *testing.T) {
	data := buildMockResponse(1013, "", false)
	text, _, done := parseASRResult(data)
	if text != "" || !done {
		t.Errorf("got text=%q done=%v, want empty/true for code 1013 (silent, session ends)", text, done)
	}
}

func TestParseASRResult_TooShort(t *testing.T) {
	_, _, done := parseASRResult([]byte{0x11, 0x00})
	if done {
		t.Error("expected done=false for short frame")
	}
}

func TestParseASRResult_ServerError(t *testing.T) {
	// msgType 0x0F in byte[1] upper nibble
	data := []byte{0x11, (msgTypeServerError << 4), 0x00, 0x00, 0, 0, 0, 42, 0, 0, 0, 0}
	_, _, done := parseASRResult(data)
	if !done {
		t.Error("expected done=true for server error frame")
	}
}

// ---- checkErrorResponse ----

func TestCheckErrorResponse_OK(t *testing.T) {
	frame := buildFrame(0x0B, flagNormal, serialJSON, compressGZP, []byte("{}"))
	if err := checkErrorResponse(frame); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckErrorResponse_ServerError(t *testing.T) {
	frame := []byte{0x11, (msgTypeServerError << 4) | 0x00, 0x00, 0x00, 0, 0, 0, 99, 0, 0, 0, 4}
	if err := checkErrorResponse(frame); err == nil {
		t.Error("expected error for server error frame")
	}
}

func TestCheckErrorResponse_TooShort(t *testing.T) {
	if err := checkErrorResponse([]byte{0x11}); err == nil {
		t.Error("expected error for too-short frame")
	}
}
