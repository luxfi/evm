//go:build !evm_node
// +build !evm_node

// Package stub provides minimal node-level interfaces when building the EVM library.
package stub

// Connector is a stand-in for the node Connector interface.
type Connector interface{}

// AppHandler is a stand-in for the node AppHandler interface.
type AppHandler interface{}

// Application is a stand-in for the node Application interface.
type Application interface{}