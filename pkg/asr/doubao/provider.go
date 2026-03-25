// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

// Package doubao implements the Doubao (火山引擎) streaming ASR provider.
// Protocol: binary WebSocket with a 4-byte header + gzip-compressed payloads.
// Docs: https://www.volcengine.com/docs/6561/1354869?lang=zh
package doubao

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/asr"
)

func init() {
	asr.Register("doubao", func(cfg map[string]any) (asr.Provider, error) {
		return newProvider(cfg)
	})
}

const defaultASRURL = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel"
const defaultASRAsyncURL = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"

// 时长包资源 ID 以控制台实际购买的为准：
// 模型1.0: volc.bigasr.sauc.duration  模型2.0(Seed): volc.seedasr.sauc.duration
const defaultResourceID = "volc.bigasr.sauc.duration"
const seedResourceID = "volc.seedasr.sauc.duration"

const (
	defaultAudioFormat   = "ogg"
	defaultAudioCodec    = "opus"
	defaultAudioRate     = 16000
	defaultAudioBits     = 16
	defaultAudioChannels = 1
)

// header byte layout (4 bytes):
//
//	[0]: (version=0x01 << 4) | header_size=0x01
//	[1]: (message_type << 4) | message_type_specific_flags
//	[2]: (serial_method << 4) | compression_type
//	[3]: reserved=0x00
const (
	msgTypeFullClientRequest = 0x01 // JSON init frame
	msgTypeAudioOnly         = 0x02 // audio frame
	msgTypeServerError       = 0x0F

	flagNormal    = 0x00
	flagLastFrame = 0x02

	serialNone  = 0x00
	serialJSON  = 0x01
	compressGZP = 0x01
)

type provider struct {
	appKey     string
	accessKey  string
	resourceID string
	wsURL      string
	dialer     *websocket.Dialer
}

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "***" + value[len(value)-4:]
}

func defaultWSURLForResource(resourceID string) string {
	if resourceID == seedResourceID {
		return defaultASRAsyncURL
	}
	return defaultASRURL
}

func newProvider(cfg map[string]any) (*provider, error) {
	appKey, _ := cfg["app_key"].(string)
	accessKey, _ := cfg["access_key"].(string)
	rid, _ := cfg["resource_id"].(string)
	wsURL, _ := cfg["ws_url"].(string)

	if appKey == "" {
		return nil, fmt.Errorf("doubao asr: app_key required")
	}
	if accessKey == "" {
		return nil, fmt.Errorf("doubao asr: access_key required")
	}
	if rid == "" {
		rid = defaultResourceID
	}
	if wsURL == "" {
		wsURL = defaultWSURLForResource(rid)
	}

	// 不走代理直连火山引擎 ASR，避免本地 http_proxy 拦截 WebSocket 连接。
	dialer := &websocket.Dialer{
		NetDialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		HandshakeTimeout: 10 * time.Second,
	}

	return &provider{appKey: appKey, accessKey: accessKey, resourceID: rid, wsURL: wsURL, dialer: dialer}, nil
}

func (p *provider) Name() string { return "doubao" }

// AudioFormat 声明 doubao ASR 默认接收 16 kHz / mono Ogg Opus 字节流。
func (p *provider) AudioFormat() asr.AudioFormat {
	return asr.AudioFormat{
		Format:     defaultAudioFormat,
		Codec:      defaultAudioCodec,
		SampleRate: defaultAudioRate,
		Channels:   defaultAudioChannels,
	}
}

// Transcribe sends frames in the declared provider format to Doubao streaming ASR
// and returns the final text.
func (p *provider) Transcribe(ctx context.Context, frames [][]byte) (string, error) {
	var final string
	err := p.transcribeInternal(ctx, frames, func(text string, isDef bool) {
		if isDef {
			final = text
		}
	})
	return final, err
}

// TranscribeStream sends frames in the declared provider format to Doubao streaming ASR and calls callback for each
// incremental result. callback is invoked only when recognized text changes, and
// final=true when recognition is complete.
func (p *provider) TranscribeStream(ctx context.Context, frames [][]byte, callback asr.ResultCallback) error {
	return p.transcribeInternal(ctx, frames, callback)
}

// connect 建立到豆包 ASR 服务的 WebSocket 连接并完成握手。
// 返回已握手的 conn，调用方负责关闭。
// ctx 取消时会异步关闭连接，使阻塞的 ReadMessage 立即返回。
func (p *provider) connect(ctx context.Context) (*websocket.Conn, error) {
	connectID := uuid.New().String()
	headers := p.authHeaders(connectID)
	log.Printf("doubao asr: dialing ws_url=%s headers={X-Api-App-Key:%s X-Api-Access-Key:%s X-Api-Resource-Id:%s X-Api-Connect-Id:%s}", p.wsURL, maskSecret(p.appKey), maskSecret(p.accessKey), p.resourceID, connectID)

	conn, resp, err := p.dialer.DialContext(ctx, p.wsURL, headers)
	if err != nil {
		if resp != nil {
			logID := resp.Header.Get("X-Tt-Logid")
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			if logID != "" {
				return nil, fmt.Errorf("doubao asr: dial: %w (HTTP %d, logid=%s: %s)", err, resp.StatusCode, logID, bytes.TrimSpace(body))
			}
			return nil, fmt.Errorf("doubao asr: dial: %w (HTTP %d: %s)", err, resp.StatusCode, bytes.TrimSpace(body))
		}
		return nil, fmt.Errorf("doubao asr: dial: %w", err)
	}

	// ctx 取消时关闭连接，使 ReadMessage 立即解除阻塞。
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	// 发送初始化帧（协议握手）。
	initReq := p.initRequest(uuid.New().String())
	frame, err := buildJSONFrame(msgTypeFullClientRequest, flagNormal, initReq)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("doubao asr: build init frame: %w", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		conn.Close()
		return nil, fmt.Errorf("doubao asr: send init: %w", err)
	}
	_, initResp, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("doubao asr: read init response: %w", err)
	}
	if err := checkErrorResponse(initResp); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// authHeaders 使用当前豆包 ASR 服务端实际接受的鉴权 header 组合。
// 线上服务除文档中的 App-Key 外，还要求额外提供 Access-Key。
func (p *provider) authHeaders(connectID string) http.Header {
	return http.Header{
		"X-Api-App-Key":     {p.appKey},
		"X-Api-Access-Key":  {p.accessKey},
		"X-Api-Resource-Id": {p.resourceID},
		"X-Api-Connect-Id":  {connectID},
	}
}

func (p *provider) initRequest(reqID string) map[string]any {
	return map[string]any{
		"user": map[string]any{"uid": "picoclaw"},
		"audio": map[string]any{
			"format":      defaultAudioFormat,
			"codec":       defaultAudioCodec,
			"rate":        defaultAudioRate,
			"bits":        defaultAudioBits,
			"channel":     defaultAudioChannels,
			"sample_rate": defaultAudioRate,
		},
		"request": map[string]any{
			"reqid":           reqID,
			"sequence":        1,
			"model_name":      "bigmodel",
			"show_utterances": true,
			"result_type":     "stream",
			"end_window_size": 200,
			"workflow":        "audio_in,resample,partition,vad,fe,decode",
		},
	}
}

// transcribeInternal is the core implementation shared by Transcribe and TranscribeStream.
// frames contain raw audio bytes in the format declared by AudioFormat().
func (p *provider) transcribeInternal(ctx context.Context, frames [][]byte, callback asr.ResultCallback) error {
	conn, err := p.connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := p.sendAudioFrames(ctx, conn, frames); err != nil {
		return err
	}

	// Read responses until we get a definite utterance or connection closes.
	var lastText string
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			break // server closed normally
		}
		text, definite, done := parseASRResult(msg)
		// 只在文本有变化时回调，消除重复片段
		if text != "" && text != lastText {
			lastText = text
			label := "partial"
			if definite {
				label = "final"
			}
			log.Printf("doubao asr: %s text=%q", label, text)
			if callback != nil {
				callback(text, definite)
			}
		}
		if done {
			break
		}
	}

	if lastText == "" {
		log.Printf("doubao asr: no final text (silent or unrecognized)")
	}
	return nil
}

func (p *provider) sendAudioFrames(ctx context.Context, conn *websocket.Conn, frames [][]byte) error {
	if len(frames) == 0 {
		audioFrame, err := buildAudioFrame(flagLastFrame, nil)
		if err != nil {
			return fmt.Errorf("doubao asr: build audio frame: %w", err)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, audioFrame); err != nil {
			return fmt.Errorf("doubao asr: send audio: %w", err)
		}
		return nil
	}

	for i, frame := range frames {
		flags := byte(flagNormal)
		if i == len(frames)-1 {
			flags = flagLastFrame
		}
		audioFrame, err := buildAudioFrame(flags, frame)
		if err != nil {
			return fmt.Errorf("doubao asr: build audio frame: %w", err)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, audioFrame); err != nil {
			return fmt.Errorf("doubao asr: send audio: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// asrResult carries the final ASR recognition result or an error.
type asrResult struct {
	text string
	err  error
}

// streamingSession is an active live doubao ASR session.
type streamingSession struct {
	conn      *websocket.Conn
	sendMu    sync.Mutex
	resultCh  chan asrResult // exactly one value written by the read goroutine
	closedCh  chan struct{}
	closeOnce sync.Once
}

// OpenSession establishes a new real-time ASR session.
// The passed context controls the session lifetime: cancellation closes the connection.
func (p *provider) OpenSession(ctx context.Context, callback asr.ResultCallback) (asr.StreamingSession, error) {
	conn, err := p.connect(ctx)
	if err != nil {
		return nil, err
	}

	sess := &streamingSession{
		conn:     conn,
		resultCh: make(chan asrResult, 1),
		closedCh: make(chan struct{}),
	}

	// 读取协程：持续接收 ASR 结果，将最终结果写入 resultCh
	go func() {
		var lastText string
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					if lastText == "" {
						sess.resultCh <- asrResult{err: asr.ErrSessionClosed}
					} else {
						sess.resultCh <- asrResult{text: lastText}
					}
				} else {
					sess.resultCh <- asrResult{err: err}
				}
				return
			}
			text, definite, done := parseASRResult(msg)
			if text != "" && text != lastText {
				lastText = text
				label := "partial"
				if definite {
					label = "final"
				}
				log.Printf("doubao asr: %s text=%q", label, text)
				if callback != nil {
					callback(text, definite)
				}
			}
			if done {
				if lastText == "" {
					log.Printf("doubao asr: no final text (silent or unrecognized)")
				}
				sess.resultCh <- asrResult{text: lastText}
				return
			}
		}
	}()

	return sess, nil
}

// SendAudio pushes a raw audio frame to the live ASR session.
// frame 格式必须与 AudioFormat() 声明一致。
func (ss *streamingSession) SendAudio(frame []byte, isLast bool) error {
	select {
	case <-ss.closedCh:
		return asr.ErrSessionClosed
	default:
	}
	flags := byte(flagNormal)
	if isLast {
		flags = flagLastFrame
	}
	audioFrame, err := buildAudioFrame(flags, frame)
	if err != nil {
		return err
	}
	ss.sendMu.Lock()
	defer ss.sendMu.Unlock()
	return ss.conn.WriteMessage(websocket.BinaryMessage, audioFrame)
}

// Wait blocks until the final ASR result is available, ctx is cancelled,
// or the session is closed.
func (ss *streamingSession) Wait(ctx context.Context) (string, error) {
	select {
	case r := <-ss.resultCh:
		return r.text, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-ss.closedCh:
		return "", asr.ErrSessionClosed
	}
}

// Close aborts the session and releases all resources.
func (ss *streamingSession) Close() {
	ss.closeOnce.Do(func() {
		close(ss.closedCh)
		ss.conn.Close()
	})
}

// buildJSONFrame wraps a JSON payload in the doubao binary frame format.
func buildJSONFrame(msgType, flags byte, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	compressed, err := gzipBytes(data)
	if err != nil {
		return nil, err
	}
	return buildFrame(msgType, flags, serialJSON, compressGZP, compressed), nil
}

// buildAudioFrame wraps raw audio bytes in the doubao binary frame format.
func buildAudioFrame(flags byte, audioBytes []byte) ([]byte, error) {
	compressed, err := gzipBytes(audioBytes)
	if err != nil {
		return nil, err
	}
	return buildFrame(msgTypeAudioOnly, flags, serialNone, compressGZP, compressed), nil
}

func buildFrame(msgType, flags, serial, compress byte, payload []byte) []byte {
	hdr := [4]byte{
		(0x01 << 4) | 0x01, // version=1, header_size=1
		(msgType << 4) | flags,
		(serial << 4) | compress,
		0x00,
	}
	frame := make([]byte, 0, 8+len(payload))
	frame = append(frame, hdr[:]...)
	frame = binary.BigEndian.AppendUint32(frame, uint32(len(payload)))
	frame = append(frame, payload...)
	return frame
}

func checkErrorResponse(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("doubao asr: response too short (%d bytes)", len(data))
	}
	if (data[1] >> 4) == msgTypeServerError {
		if len(data) >= 8 {
			code := binary.BigEndian.Uint32(data[4:8])
			return fmt.Errorf("doubao asr: server error code=%d", code)
		}
		return fmt.Errorf("doubao asr: server error")
	}
	return nil
}

type asrPayload struct {
	Code   int `json:"code"`
	Result struct {
		Utterances []struct {
			Text     string `json:"text"`
			Definite bool   `json:"definite"`
		} `json:"utterances"`
	} `json:"result"`
}

// parseASRResult parses a server response frame, handling optional gzip compression.
// Returns (text, definite, done).
//   - definite=false → 中间结果，text 可能非空（继续等待后续帧）
//   - definite=true  → 最终结果，text 非空，done=true
func parseASRResult(data []byte) (text string, definite bool, done bool) {
	if len(data) < 12 {
		return "", false, false
	}
	if (data[1] >> 4) == msgTypeServerError {
		return "", false, true
	}
	// byte 2 lower nibble = compression type: 0x01 = gzip
	payload := data[12:]
	if data[2]&0x0F == compressGZP {
		uncompressed, err := gunzipBytes(payload)
		if err != nil {
			log.Printf("doubao asr: decompress response: %v", err)
			return "", false, false
		}
		payload = uncompressed
	}
	var p asrPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		log.Printf("doubao asr: parse response JSON: %v", err)
		return "", false, false
	}
	if p.Code == 1013 { // no speech detected
		return "", false, true
	}
	if p.Code != 0 && p.Code != 1000 {
		log.Printf("doubao asr: server code=%d", p.Code)
	}
	for _, u := range p.Result.Utterances {
		if u.Text != "" {
			if u.Definite {
				return u.Text, true, true
			}
			// 返回中间结果，不打 log（由调用方在文本变化时记录）
			return u.Text, false, false
		}
	}
	return "", false, false
}

func gunzipBytes(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
