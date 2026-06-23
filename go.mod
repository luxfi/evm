module github.com/luxfi/evm

go 1.26.4

exclude github.com/luxfi/geth v1.16.1

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/deckarep/golang-set/v2 v2.9.0
	github.com/fjl/gencodec v0.1.1
	github.com/gorilla/rpc v1.2.1
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674
	github.com/hashicorp/go-bexpr v0.1.16
	github.com/hashicorp/golang-lru v1.0.2
	github.com/holiman/billy v0.0.0-20250707135307-f2f9b9aae7db
	github.com/holiman/bloomfilter/v2 v2.0.3
	github.com/holiman/uint256 v1.3.2
	github.com/luxfi/cache v1.2.1
	github.com/luxfi/chains v1.3.18
	github.com/luxfi/consensus v1.25.19
	github.com/luxfi/constants v1.5.8
	github.com/luxfi/crypto v1.19.21
	github.com/luxfi/database v1.20.4
	github.com/luxfi/dex v1.5.16
	github.com/luxfi/filesystem v0.0.1
	github.com/luxfi/geth v1.17.12
	github.com/luxfi/go-bip39 v1.1.2
	github.com/luxfi/gpu v1.0.1
	github.com/luxfi/ids v1.2.15
	github.com/luxfi/log v1.4.3
	github.com/luxfi/math v1.4.1
	github.com/luxfi/math/big v0.1.0
	github.com/luxfi/math/safe v0.0.1
	github.com/luxfi/metric v1.5.9
	github.com/luxfi/p2p v1.21.1
	github.com/luxfi/pq v1.0.3
	github.com/luxfi/precompile v0.5.58
	github.com/luxfi/runtime v1.1.3
	github.com/luxfi/sys v0.1.0
	github.com/luxfi/timer v1.0.2
	github.com/luxfi/trace v1.1.0
	github.com/luxfi/upgrade v1.0.1
	github.com/luxfi/utils v1.2.0
	github.com/luxfi/validators v1.2.0
	github.com/luxfi/version v1.0.1
	github.com/luxfi/vm v1.2.5
	github.com/luxfi/warp v1.19.5
	github.com/mattn/go-colorable v0.1.15
	github.com/mattn/go-isatty v0.0.22
	github.com/spf13/cast v1.10.0
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/urfave/cli/v2 v2.27.7
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
	go.uber.org/goleak v1.3.0
	go.uber.org/mock v0.6.0
	golang.org/x/crypto v0.52.0
	golang.org/x/exp v0.0.0-20260529124908-c761662dc8c9
	golang.org/x/mod v0.36.0
	golang.org/x/sync v0.20.0
	golang.org/x/time v0.15.0
	golang.org/x/tools v0.45.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	filippo.io/hpke v0.4.0 // indirect
	github.com/ChainSafe/go-schnorrkel v1.1.0 // indirect
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2 v1.41.5 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.5 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.13 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.13 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.6 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.96.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.10 // indirect
	github.com/aws/smithy-go v1.24.2 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cockroachdb/errors v1.12.0 // indirect
	github.com/cockroachdb/fifo v0.0.0-20240816210425-c5d0cb0b6fc0 // indirect
	github.com/cockroachdb/logtags v0.0.0-20241215232642-bb51bb14a506 // indirect
	github.com/cockroachdb/pebble v1.1.5 // indirect
	github.com/cockroachdb/redact v1.1.8 // indirect
	github.com/cockroachdb/tokenbucket v0.0.0-20250429170803-42689b6311bb // indirect
	github.com/cosmos/go-bip39 v1.0.0 // indirect
	github.com/cronokirby/saferith v0.33.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.1 // indirect
	github.com/getsentry/sentry-go v0.44.1 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/renameio/v2 v2.0.2 // indirect
	github.com/grandcat/zeroconf v1.0.0 // indirect
	github.com/gtank/merlin v0.1.1 // indirect
	github.com/gtank/ristretto255 v0.2.0 // indirect
	github.com/hanzoai/vfs v0.4.3 // indirect
	github.com/hanzos3/go-sdk v1.0.2 // indirect
	github.com/klauspost/crc32 v1.3.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/luxfi/accel v1.2.4 // indirect
	github.com/luxfi/age v1.5.0 // indirect
	github.com/luxfi/ai v0.1.0 // indirect
	github.com/luxfi/api v1.0.15 // indirect
	github.com/luxfi/atomic v1.0.0 // indirect
	github.com/luxfi/bft v0.1.5 // indirect
	github.com/luxfi/compress v0.0.5 // indirect
	github.com/luxfi/concurrent v0.0.3 // indirect
	github.com/luxfi/container v0.0.4 // indirect
	github.com/luxfi/corona v0.7.9 // indirect
	github.com/luxfi/crypto/ipa v1.2.4 // indirect
	github.com/luxfi/fhe v1.8.2 // indirect
	github.com/luxfi/lattice/v7 v7.1.4 // indirect
	github.com/luxfi/mdns v0.1.1 // indirect
	github.com/luxfi/proto v1.3.5 // indirect
	github.com/luxfi/resource v0.0.1 // indirect
	github.com/luxfi/threshold v1.9.9 // indirect
	github.com/luxfi/zap v0.8.10 // indirect
	github.com/luxfi/zapdb v1.10.1 // indirect
	github.com/miekg/dns v1.1.72 // indirect
	github.com/mimoo/StrobeGo v0.0.0-20220103164710-9a04d6ca976b // indirect
	github.com/minio/crc64nvme v1.1.1 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.mongodb.org/mongo-driver v1.17.9 // indirect
)

require (
	github.com/ALTree/bigfloat v0.2.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.24.4 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/consensys/gnark-crypto v0.20.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/crate-crypto/go-eth-kzg v1.5.0 // indirect
	github.com/crate-crypto/go-kzg-4844 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/dgraph-io/ristretto/v2 v2.4.0 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/dot v1.11.0 // indirect
	github.com/ethereum/c-kzg-4844/v2 v2.1.7 // indirect
	github.com/ethereum/go-bigmodexpfix v0.0.0-20250911101455-f9e208c548ab // indirect
	github.com/ferranbt/fastssz v1.0.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/garslo/gogen v0.0.0-20170306192744-1d203ffc1f61 // indirect
	github.com/gballet/go-libpcsclite v0.0.0-20250918194357-1ec6f2e601c6 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20260302011040-a15ffb7f9dcc // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/huin/goupnp v1.3.0 // indirect
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/luxfi/formatting v1.0.1
	github.com/luxfi/mock v0.1.1 // indirect
	github.com/luxfi/sampler v1.1.0 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/pointerstructure v1.2.1 // indirect
	github.com/montanaflynn/stats v0.9.0 // indirect
	github.com/mr-tron/base58 v1.3.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pelletier/go-toml/v2 v2.3.0 // indirect
	github.com/pion/dtls/v2 v2.2.12 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/stun/v2 v2.0.0 // indirect
	github.com/pion/transport/v2 v2.2.10 // indirect
	github.com/pion/transport/v3 v3.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/status-im/keycard-go v0.3.3 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/supranational/blst v0.3.16 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

exclude (
	github.com/luxfi/precompiles v0.1.10
	github.com/luxfi/precompiles v0.1.11
)
