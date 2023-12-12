// Copyright (C) 2019-2022, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import "time"

const (
	// Timeout to boot the LuxGo node
	BootLuxNodeTimeout = 5 * time.Minute

	// Timeout for the health API to check the LuxGo is ready
	HealthCheckTimeout = 5 * time.Second

	DefaultLocalNodeURI = "http://127.0.0.1:9650"
)

var (
	NodeURIs = []string{DefaultLocalNodeURI, "http://127.0.0.1:9652", "http://127.0.0.1:9654", "http://127.0.0.1:9656", "http://127.0.0.1:9658"}
)
