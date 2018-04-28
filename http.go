package introspection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	discoveryPath = ".well-known/openid-configuration"
)

var (
	// ErrNoBearer is returned by FromContext function when no Bearer token was present
	ErrNoBearer = errors.New("no bearer")
	// ErrNoMiddleware is returned by FromContext when no value was set. It is due to the middleware not being called before this function.
	ErrNoMiddleware = errors.New("introspection middleware didn't execute")
)

// Introspection ...
func Introspection(endpoint string, opts ...Option) func(http.Handler) http.Handler {

	opt := &Options{
		Client: &http.Client{
			Timeout: 2 * time.Second,
		},
		body:   url.Values{"token": {""}, "token_type_hint": {"access_token"}},
		header: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Accept": {"application/json"}},

		endpoint: endpoint,
	}

	for _, apply := range opts {
		apply(opt)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hd := r.Header.Get("Authorization")

			if !strings.HasPrefix(hd, "Bearer ") {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), resKey, &result{Err: ErrNoBearer})))
				return
			}

			token := hd[len("Bearer "):]

			if opt.cache != nil {
				if res := opt.cache.Get(token); res != nil {
					next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), resKey, &result{res, nil})))
					return
				}
			}

			res, err := introspect(token, *opt)

			if err == nil && opt.cache != nil {
				opt.cache.Store(token, res, opt.cacheExp)
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), resKey, &result{res, err})))
		})
	}
}

type resKeyType int

const resKey = resKeyType(1)

type result struct {
	Result *Result
	Err    error
}

// FromContext ...
func FromContext(ctx context.Context) (*Result, error) {
	if val, ok := ctx.Value(resKey).(*result); ok {
		return val.Result, val.Err
	}

	return nil, ErrNoMiddleware
}
