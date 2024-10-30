// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build integration && !windows

package go_elasticsearch

import (
	"context"
	"io"
	"testing"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/stretchr/testify/require"
)

type TestCaseV7 struct {
	base
}

func (tc *TestCaseV7) Setup(t *testing.T) {
	tc.base.Setup(t, "docker.elastic.co/elasticsearch/elasticsearch:7.17.24", func(addr string, _ []byte) (esClient, error) {
		return elasticsearch.NewClient(elasticsearch.Config{
			Addresses: []string{addr},
		})
	})
}

func (tc *TestCaseV7) Run(t *testing.T) {
	tc.base.Run(t, func(t *testing.T, client esClient, body io.Reader) {
		t.Helper()
		req := esapi.IndexRequest{
			Index:      "test",
			DocumentID: "1",
			Body:       body,
			Refresh:    "true",
		}
		res, err := req.Do(context.Background(), client)
		require.NoError(t, err)
		defer res.Body.Close()
	})
}