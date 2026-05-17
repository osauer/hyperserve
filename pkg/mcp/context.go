package mcp

import "context"

// ToolWithContext is an enhanced Tool that supports context for cancellation
// and timeouts during ExecuteWithContext.
type ToolWithContext interface {
	Tool
	ExecuteWithContext(ctx context.Context, params map[string]any) (any, error)
}

// wrapToolWithContext wraps a Tool so that it satisfies ToolWithContext. If
// the input already implements ToolWithContext, it is returned unchanged.
func wrapToolWithContext(tool Tool) ToolWithContext {
	if ctxTool, ok := tool.(ToolWithContext); ok {
		return ctxTool
	}
	return &contextToolWrapper{tool: tool}
}

type contextToolWrapper struct {
	tool Tool
}

func (w *contextToolWrapper) Name() string           { return w.tool.Name() }
func (w *contextToolWrapper) Description() string    { return w.tool.Description() }
func (w *contextToolWrapper) Schema() map[string]any { return w.tool.Schema() }
func (w *contextToolWrapper) Execute(params map[string]any) (any, error) {
	return w.tool.Execute(params)
}

func (w *contextToolWrapper) ExecuteWithContext(ctx context.Context, params map[string]any) (any, error) {
	type result struct {
		value any
		err   error
	}
	resultChan := make(chan result, 1)
	go func() {
		value, err := w.tool.Execute(params)
		resultChan <- result{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		return res.value, res.err
	}
}
