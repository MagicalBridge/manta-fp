package service

import (
	"fmt"

	"github.com/Manta-Network/manta-fp/types"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto/tmhash"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (fp *FinalityProviderInstance) getPubRandList(startHeight uint64, numPubRand uint32) ([]*btcec.FieldVal, error) {
	pubRandList, err := fp.em.CreateRandomnessPairList(
		fp.btcPk.MustMarshal(),
		fp.GetChainID(),
		startHeight,
		numPubRand,
		fp.passphrase,
	)
	if err != nil {
		return nil, err
	}

	return pubRandList, nil
}

// TODO: have this function in Babylon side
func getHashToSignForCommitPubRand(startHeight uint64, numPubRand uint64, commitment []byte) ([]byte, error) {
	hasher := tmhash.New()
	if _, err := hasher.Write(sdk.Uint64ToBigEndian(startHeight)); err != nil {
		return nil, err
	}
	if _, err := hasher.Write(sdk.Uint64ToBigEndian(numPubRand)); err != nil {
		return nil, err
	}
	if _, err := hasher.Write(commitment); err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

func (fp *FinalityProviderInstance) signPubRandCommit(startHeight uint64, numPubRand uint64, commitment []byte) (*schnorr.Signature, error) {
	hash, err := getHashToSignForCommitPubRand(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
	}

	// sign the message hash using the bbn-fp's BTC private key
	return fp.em.SignSchnorrSig(fp.btcPk.MustMarshal(), hash, fp.passphrase)
}

// TODO: have this function in Babylon side
func getMsgToSignForVote(blockHeight uint64, stateRoot []byte) []byte {
	return append(sdk.Uint64ToBigEndian(blockHeight), stateRoot...)
}

func (fp *FinalityProviderInstance) signFinalitySig(b *types.BlockInfo) (*bbntypes.SchnorrEOTSSig, error) {
	// build proper finality signature request
	msgToSign := getMsgToSignForVote(b.L2BlockNumber.Uint64(), b.StateRoot.StateRoot[:])
	sig, err := fp.em.SignEOTS(fp.btcPk.MustMarshal(), fp.GetChainID(), msgToSign, b.L2BlockNumber.Uint64(), fp.passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	return bbntypes.NewSchnorrEOTSSigFromModNScalar(sig), nil
}
