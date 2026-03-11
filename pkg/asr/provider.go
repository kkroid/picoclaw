// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package asr

import (
	"context"
	"errors"
	"fmt"
)

// Provider is the ASR interface.
// Audio format: 16kHz 16-bit mono PCM (全局约定不传 sampleRate).
type Provider interface {
	Name() string
	// Transcribe converts PCM audio to text.
	// pcm: 16kHz 16-bit mono samples (端侧 VAD 已过滤，只传有效语音段)
	Transcribe(ctx context.Context, pcm []int16) (string, error)
}

// ResultCallback receives incremental ASR results.
// final=true 表示识别完成（definite），后续不再有回调。
type ResultCallback func(text string, final bool)

// StreamingProvider extends Provider with streaming (incremental) ASR support.
// The callback is invoked each time the recognized text changes, and once more
// with final=true when recognition is complete.
type StreamingProvider interface {
	Provider
	TranscribeStream(ctx context.Context, pcm []int16, callback ResultCallback) error
}

// ErrSessionClosed is returned by StreamingSession.Wait when the session
// was closed before a final result was available.
var ErrSessionClosed = errors.New("asr: session closed")

// StreamingSession is an active real-time ASR session.
// Open with RealtimeProvider.OpenSession; feed audio with SendAudio;
// signal end-of-audio by passing isLast=true; then call Wait for the result.
type StreamingSession interface {
	// SendAudio pushes a PCM chunk. Set isLast=true on the final chunk.
	SendAudio(pcm []int16, isLast bool) error
	// Wait blocks until the final recognition result is available.
	Wait(ctx context.Context) (string, error)
	// Close aborts the session and releases all resources.
	Close()
}

// RealtimeProvider extends Provider with live frame-by-frame ASR.
// Audio is fed incrementally as it arrives, reducing latency.
type RealtimeProvider interface {
	Provider
	// OpenSession starts a new live ASR session.
	// callback receives each incremental result (final=true on completion).
	OpenSession(ctx context.Context, callback ResultCallback) (StreamingSession, error)
}

// Factory creates a Provider from a config map.
type Factory func(cfg map[string]any) (Provider, error)

var factories = map[string]Factory{}

// Register adds a provider factory. Called from provider init() functions.
func Register(name string, f Factory) {
	factories[name] = f
}

// New creates a Provider by name.
func New(name string, cfg map[string]any) (Provider, error) {
	f, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("asr: provider %q not registered", name)
	}
	return f(cfg)
}
