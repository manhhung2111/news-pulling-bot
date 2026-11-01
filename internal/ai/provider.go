package ai

import (
	"context"
	"errors"
	"fmt"
)

type Provider interface {
	Name() string
	Complete(ctx context.Context, prompt string) (string, error)
}

// Chain tries each provider in order until one succeeds.
type Chain struct{ Providers []Provider }

// Complete returns the first successful provider's text and name.
func (c Chain) Complete(ctx context.Context, prompt string) (text, provider string, err error) {
	var errs []error
	for _, p := range c.Providers {
		text, err := p.Complete(ctx, prompt)
		if err == nil {
			return text, p.Name(), nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", p.Name(), err))
	}
	if len(errs) == 0 {
		return "", "", errors.New("no AI providers configured")
	}
	return "", "", errors.Join(errs...)
}

// CompleteParsed tries each provider in order. A reply that fails parse is retried once with the same
// provider, then counts as that provider's failure and the next provider is tried.
func (c Chain) CompleteParsed(ctx context.Context, prompt string, parse func(string) error) (provider string, err error) {
	var errs []error
	for _, p := range c.Providers {
		for attempt := 0; attempt < 2; attempt++ {
			text, err := p.Complete(ctx, prompt)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", p.Name(), err))
				break
			}
			perr := parse(text)
			if perr == nil {
				return p.Name(), nil
			}
			if attempt == 1 {
				errs = append(errs, fmt.Errorf("%s: %w", p.Name(), perr))
			}
		}
	}
	if len(errs) == 0 {
		return "", errors.New("no AI providers configured")
	}
	return "", errors.Join(errs...)
}
