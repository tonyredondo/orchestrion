// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build integration

package gin

import (
	"context"
	"net/http"
	"testing"
	"time"

	"datadoghq.dev/orchestrion/_integration-tests/utils"
	"datadoghq.dev/orchestrion/_integration-tests/validator/trace"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestCase struct {
	*http.Server
}

func (tc *TestCase) Setup(t *testing.T) {
	gin.SetMode(gin.ReleaseMode) // Silence start-up logging
	engine := gin.New()

	tc.Server = &http.Server{
		Addr:    "127.0.0.1:" + utils.GetFreePort(t),
		Handler: engine.Handler(),
	}

	engine.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })

	go func() { assert.ErrorIs(t, tc.Server.ListenAndServe(), http.ErrServerClosed) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		assert.NoError(t, tc.Server.Shutdown(ctx))
	})
}

func (tc *TestCase) Run(t *testing.T) {
	resp, err := http.Get("http://" + tc.Server.Addr + "/ping")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func (tc *TestCase) ExpectedTraces() trace.Traces {
	httpUrl := "http://" + tc.Server.Addr + "/ping"
	return trace.Traces{
		{
			// NB: 2 Top-level spans are from the HTTP Client/Server, which are library-side instrumented.
			Tags: map[string]any{
				"name":     "http.request",
				"resource": "GET /ping",
				"service":  "gin.test",
				"type":     "http",
			},
			Meta: map[string]string{
				"http.url":  httpUrl,
				"component": "net/http",
				"span.kind": "client",
			},
			Children: trace.Traces{
				{
					Tags: map[string]any{
						"name":     "http.request",
						"resource": "GET /ping",
						"service":  "gin.test",
						"type":     "web",
					},
					Meta: map[string]string{
						"http.url":  httpUrl,
						"component": "net/http",
						"span.kind": "server",
					},

					Children: trace.Traces{
						{
							Tags: map[string]any{
								"name":     "http.request",
								"resource": "GET /ping",
								"service":  "gin.router",
								"type":     "web",
							},
							Meta: map[string]string{
								"http.url":  httpUrl,
								"component": "gin-gonic/gin",
								"span.kind": "server",
							},
						},
					},
				},
			},
		},
	}
}
