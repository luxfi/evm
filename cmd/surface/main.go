// Command surface prints the precompile surface a luxd built from this module
// links: one line per registered module, address and config key. A chain can
// only activate a configKey that appears here, so this is the list an
// upgrade.json is allowed to name.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/luxfi/precompile/modules"
	_ "github.com/luxfi/evm/precompile/registry"
)

func main() {
	all := modules.RegisteredModules()
	rows := make([]string, 0, len(all))
	for _, m := range all {
		rows = append(rows, fmt.Sprintf("%s  %s", m.Address.Hex(), m.ConfigKey))
	}
	sort.Strings(rows)
	for _, r := range rows {
		fmt.Println(r)
	}
	fmt.Fprintf(os.Stderr, "%d modules\n", len(rows))
}
