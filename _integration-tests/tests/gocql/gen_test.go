// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//
// Code generated by 'go generate'; DO NOT EDIT.

//go:build integration

package gocql

import (
	utils "orchestrion/integration/utils"
	"testing"
)

func TestIntegration_gocql_NewCluster(t *testing.T) {
	utils.RunTest(t, new(TestCaseNewCluster))
}

func TestIntegration_gocql_StructLiteral(t *testing.T) {
	utils.RunTest(t, new(TestCaseStructLiteral))
}

func TestIntegration_gocql_StructLiteralPtr(t *testing.T) {
	utils.RunTest(t, new(TestCaseStructLiteralPtr))
}