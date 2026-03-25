// [KKROID FORK] 流式回调通过 context 透传到 provider 层，
// 使 agent 主循环无需感知流式逻辑。
package streamctx

import "context"

type streamCallbackKey struct{}

// WithCallback 将流式 token 回调注入 context。
// provider 的 Chat() 方法检测到该回调后自动升级为 SSE 流式调用。
func WithCallback(ctx context.Context, cb func(string)) context.Context {
	return context.WithValue(ctx, streamCallbackKey{}, cb)
}

// GetCallback 从 context 中提取流式回调。
func GetCallback(ctx context.Context) (func(string), bool) {
	cb, ok := ctx.Value(streamCallbackKey{}).(func(string))
	return cb, ok && cb != nil
}
