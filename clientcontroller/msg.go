package clientcontroller

type CommitPublicRandomnessMsg struct {
	CommitPublicRandomness CommitPublicRandomnessMsgParams `json:"commit_public_randomness"`
}

type CommitPublicRandomnessMsgParams struct {
	FpPubkeyHex string `json:"fp_pubkey_hex"`
	StartHeight uint64 `json:"start_height"`
	NumPubRand  uint64 `json:"num_pub_rand"`
	Commitment  []byte `json:"commitment"`
	Signature   []byte `json:"signature"`
}

// TODO: need to update based on contract implementation
type CommitPublicRandomnessResponse struct {
	Result bool `json:"result"`
}

type SubmitFinalitySignatureMsg struct {
	SubmitFinalitySignature SubmitFinalitySignatureMsgParams `json:"submit_finality_signature"`
}

type SubmitFinalitySignatureMsgParams struct {
	FpPubkeyHex   string `json:"fp_pubkey_hex"`
	L1BlockNumber uint64 `json:"l1_block_number"`
	L1BlockHash   string `json:"l1_block_hash"`
	L2BlockNumber uint64 `json:"l2_block_number"`
	PubRand       []byte `json:"pub_rand"`
	Proof         Proof  `json:"proof"`
	StateRoot     []byte `json:"state_root"`
	Signature     []byte `json:"signature"`
}

// TODO: need to update based on contract implementation
type SubmitFinalitySignatureResponse struct {
	Result bool `json:"result"`
}

type QueryMsg struct {
	Config             *Config        `json:"config,omitempty"`
	FirstPubRandCommit *PubRandCommit `json:"first_pub_rand_commit,omitempty"`
	LastPubRandCommit  *PubRandCommit `json:"last_pub_rand_commit,omitempty"`
}

type Config struct{}

type PubRandCommit struct {
	BtcPkHex string `json:"btc_pk_hex"`
}

type ConfigResponse struct {
	ConsumerId      string `json:"consumer_id"`
	ActivatedHeight uint64 `json:"activated_height"`
}

// FIXME: Remove this ancillary struct.
// Only required because the e2e tests are using a zero index, which is removed by the `json:"omitempty"` annotation in
// the original cmtcrypto Proof
type Proof struct {
	Total    uint64   `json:"total"`
	Index    uint64   `json:"index"`
	LeafHash []byte   `json:"leaf_hash"`
	Aunts    [][]byte `json:"aunts"`
}
