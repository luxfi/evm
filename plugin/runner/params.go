// (c) 2019-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package runner

import (
	"flag"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func evmFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("evm", flag.ContinueOnError)

	fs.Bool(versionKey, false, "If true, print version and quit")

	return fs
}

// getViper returns the viper environment for the plugin binary
func getViper() (*viper.Viper, error) {
	v := viper.New()

	fs := evmFlagSet()
	pflag.CommandLine.AddGoFlagSet(fs)
	pflag.Parse()
	if err := v.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}

	return v, nil
}

func PrintVersion() (bool, error) {
	v, err := getViper()
	if err != nil {
		return false, err
	}

	if v.GetBool(versionKey) {
		return true, nil
	}
	return false, nil
}
