// Copyright (C) 2020-2022, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"github.com/luxfi/geth/log"
	"github.com/go-cmd/cmd"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"github.com/luxfi/node/api/health"
)

// RunCommand starts the command [bin] with the given [args] and returns the command to the caller
// TODO cmd package mentions we can do this more efficiently with cmd.NewCmdOptions rather than looping
// and calling Status().
func RunCommand(bin string, args ...string) (*cmd.Cmd, error) {
	log.Info("Executing", "cmd", fmt.Sprintf("%s %s", bin, strings.Join(args, " ")))

	curCmd := cmd.NewCmd(bin, args...)
	_ = curCmd.Start()

	// to stream outputs
	ticker := time.NewTicker(10 * time.Millisecond)
	go func() {
		prevLine := ""
		for range ticker.C {
			status := curCmd.Status()
			n := len(status.Stdout)
			if n == 0 {
				continue
			}

			line := status.Stdout[n-1]
			if prevLine != line && line != "" {
				fmt.Println("[streaming output]", line)
			}

			prevLine = line
		}
	}()

	return curCmd, nil
}

func RegisterPingTest() {
	require := require.New(ginkgo.GinkgoT())

	ginkgo.It("ping the network", ginkgo.Label("ping"), func() {
		client := health.NewClient(DefaultLocalNodeURI)
		healthyReply, err := client.Health(context.Background(), nil)
		require.NoError(err)
		require.NotNil(healthyReply)
		require.True(healthyReply.Healthy)
	})
}

// RegisterNodeRun registers a before suite that starts a Lux process to use for the e2e tests
// and an after suite that stops the Lux process
func RegisterNodeRun() {
	// BeforeSuite starts a Lux process to use for the e2e tests
	var startCmd *cmd.Cmd
	_ = ginkgo.BeforeSuite(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		wd, err := os.Getwd()
		gomega.Expect(err).Should(gomega.BeNil())
		log.Info("Starting Lux node", "wd", wd)
		cmd, err := RunCommand("./scripts/run.sh")
		startCmd = cmd
		gomega.Expect(err).Should(gomega.BeNil())

		// Assumes that startCmd will launch a node with HTTP Port at [utils.DefaultLocalNodeURI]
		healthClient := health.NewClient(DefaultLocalNodeURI)
		
		// Wait for the node to be healthy
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(HealthCheckTimeout)
		
		for {
			select {
			case <-ticker.C:
				healthyReply, err := healthClient.Health(ctx, nil)
				if err == nil && healthyReply != nil && healthyReply.Healthy {
					gomega.Expect(true).Should(gomega.BeTrue())
					return
				}
			case <-timeout:
				gomega.Expect(false).Should(gomega.BeTrue(), "Node did not become healthy within timeout")
				return
			}
		}
		log.Info("Lux node is healthy")
	})

	ginkgo.AfterSuite(func() {
		gomega.Expect(startCmd).ShouldNot(gomega.BeNil())
		gomega.Expect(startCmd.Stop()).Should(gomega.Succeed())
		// TODO add a new node to bootstrap off of the existing node and ensure it can bootstrap all subnets
		// created during the test
	})
}

// RunHardhatTests runs the hardhat tests in the given [testPath] on the blockchain with [blockchainID]
// [execPath] is the path where the test command is executed
func RunHardhatTests(ctx context.Context, blockchainID string, execPath string, testPath string) {
	chainURI := GetDefaultChainURI(blockchainID)
	RunHardhatTestsCustomURI(ctx, chainURI, execPath, testPath)
}

func RunHardhatTestsCustomURI(ctx context.Context, chainURI string, execPath string, testPath string) {
	require := require.New(ginkgo.GinkgoT())

	log.Info(
		"Executing HardHat tests on blockchain",
		"testPath", testPath,
		"ChainURI", chainURI,
	)

	cmd := exec.Command("npx", "hardhat", "test", testPath, "--network", "local")
	cmd.Dir = execPath

	log.Info("Sleeping to wait for test ping", "rpcURI", chainURI)
	err := os.Setenv("RPC_URI", chainURI)
	require.NoError(err)
	log.Info("Running test command", "cmd", cmd.String())

	out, err := cmd.CombinedOutput()
	fmt.Printf("\nCombined output:\n\n%s\n", string(out))
	require.NoError(err)
}
