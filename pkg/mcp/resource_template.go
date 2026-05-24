package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type namespacedResourceTemplate struct {
	namespace string
	prefix    string
	template  ResourceTemplate
}

func (t *namespacedResourceTemplate) URITemplate() string {
	return t.prefix + t.template.URITemplate()
}

func (t *namespacedResourceTemplate) Name() string        { return t.template.Name() }
func (t *namespacedResourceTemplate) Description() string { return t.template.Description() }
func (t *namespacedResourceTemplate) MimeType() string    { return t.template.MimeType() }

func (t *namespacedResourceTemplate) Match(uri string) (map[string]string, bool) {
	rawURI, ok := strings.CutPrefix(uri, t.prefix)
	if !ok {
		return nil, false
	}
	return t.template.Match(rawURI)
}

func (t *namespacedResourceTemplate) Read(ctx context.Context, uri string, params map[string]string) (any, error) {
	rawURI, ok := strings.CutPrefix(uri, t.prefix)
	if !ok {
		return nil, fmt.Errorf("resource URI %q does not match namespace %q", uri, t.namespace)
	}
	return t.template.Read(ctx, rawURI, params)
}

type namespacedSubscribableResourceTemplate struct {
	*namespacedResourceTemplate
	template SubscribableResourceTemplate
}

func (t *namespacedSubscribableResourceTemplate) Subscribe(ctx context.Context, uri string, params map[string]string, emit ResourceEmitter) error {
	rawURI, ok := strings.CutPrefix(uri, t.prefix)
	if !ok {
		return fmt.Errorf("resource URI %q does not match namespace %q", uri, t.namespace)
	}
	return t.template.Subscribe(ctx, rawURI, params, &namespacedResourceEmitter{
		prefix: t.prefix,
		emit:   emit,
	})
}

type namespacedResourceEmitter struct {
	prefix string
	emit   ResourceEmitter
}

func (e *namespacedResourceEmitter) Update(uri string) error {
	if strings.HasPrefix(uri, e.prefix) {
		return e.emit.Update(uri)
	}
	return e.emit.Update(e.prefix + uri)
}

func resourceContent(uri, mimeType string, content any) (ResourceContent, error) {
	textContent, err := resourceText(content)
	if err != nil {
		return ResourceContent{}, err
	}
	return ResourceContent{URI: uri, MimeType: mimeType, Text: textContent}, nil
}

func resourceText(content any) (string, error) {
	switch v := content.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("failed to marshal resource content to JSON: %w", err)
		}
		return string(jsonBytes), nil
	}
}
