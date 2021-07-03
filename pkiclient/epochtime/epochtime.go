package epochtime

import (
	"context"
	"time"

	kpki "github.com/hashcloak/Meson-client/pkiclient"
)

//! The duration of a katzenmint epoch. Should refer to katzenmint PKI.
var TestPeriod = 20 * time.Minute

func Now(client kpki.Client) (epoch uint64, startHeight int64, err error) {
	return client.GetEpoch(context.Background())
}
