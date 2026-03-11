// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package tts

import (
	"context"
	"fmt"
)

// Provider is the TTS interface.
// Output: 16kHz 16-bit mono PCM chunks via onChunk callback.
// 音频格式全局约定：16kHz 16-bit mono，调用方负责 Opus 编码后推给客户端。
type Provider interface {
	Name() string
	// SynthesizeStream converts text to speech and delivers PCM chunks via onChunk.
	// voice: 音色名称，空字符串使用 provider 默认值.
	SynthesizeStream(ctx context.Context, text, voice string, onChunk func([]int16)) error
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
		return nil, fmt.Errorf("tts: provider %q not registered", name)
	}
	return f(cfg)
}
