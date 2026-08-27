package signal

import "encoding/json"

// Envelope is the top-level publication from the Signal Stream.
// See: https://docs.memecatcher.bubudev.win/signal-stream
type Envelope struct {
	V       int             `json:"v"`
	ID      string          `json:"id"`
	Kind    string          `json:"kind"`
	Ts      float64         `json:"ts"`     // ms epoch; API sometimes sends fractional ms
	WallTs  float64         `json:"wallTs"` // ms epoch; API sometimes sends fractional ms
	Mint    string          `json:"mint,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

// Payload holds common and kind-specific fields. Unknown fields are ignored
// when decoding; keep RawMessage on Envelope if you need the full original.
type Payload struct {
	Kind      string  `json:"kind"`
	Symbol    string  `json:"symbol,omitempty"`
	McapUsd   float64 `json:"mcapUsd,omitempty"`
	Level     int     `json:"level,omitempty"`
	Tier      string  `json:"tier,omitempty"`      // out
	Trigger   string  `json:"trigger,omitempty"`   // out
	MidMcap   float64 `json:"midMcap,omitempty"`   // out
	EntryMcap float64 `json:"entryMcap,omitempty"` // out
	PeakMcap  float64 `json:"peakMcap,omitempty"`  // out
	Detail    string  `json:"detail,omitempty"`    // out
}

// Kind constants from the Signal Stream docs.
const (
	KindArm        = "arm"
	KindArmPreMig  = "arm_pre_mig"
	KindPump       = "pump"
	KindMigrate    = "migrate"
	KindMilestone  = "milestone"
	KindWhale      = "whale"
	KindWhaleBurst = "whale_burst"
	KindWhaleArmed = "whale_armed"
	KindBcMid      = "bc_mid"
	KindOut        = "out"
)

func DecodePayload(raw json.RawMessage) (Payload, error) {
	var p Payload
	if len(raw) == 0 {
		return p, nil
	}
	err := json.Unmarshal(raw, &p)
	return p, err
}
