package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Manta-Network/manta-fp/eotsmanager"
	"github.com/Manta-Network/manta-fp/eotsmanager/config"
	"github.com/Manta-Network/manta-fp/log"
	"github.com/Manta-Network/manta-fp/util"

	"github.com/babylonlabs-io/babylon/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	cryptokeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

type KeyOutputWithPubKeyHex struct {
	keys.KeyOutput
	PubKeyHex string `json:"pubkey_hex" yaml:"pubkey_hex"`
}

func NewKeysCmd() *cobra.Command {
	keysCmd := keys.Commands()

	// Find the "add" subcommand
	addCmd := util.GetSubCommand(keysCmd, "add")
	if addCmd == nil {
		panic("failed to find keys add command")
	}

	// Override the original RunE function to run almost the same as
	// the sdk, but it allows empty hd path and allow to save the key
	// in the name mapping
	addCmd.RunE = func(cmd *cobra.Command, args []string) error {
		oldOut := cmd.OutOrStdout()

		// Create a buffer to intercept the key items
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		// Run the original command
		err := runAddCmdPrepare(cmd, args)
		if err != nil {
			return err
		}

		cmd.SetOut(oldOut)
		return saveKeyNameMapping(cmd, args)
	}

	return keysCmd
}

func saveKeyNameMapping(cmd *cobra.Command, args []string) error {
	clientCtx, err := client.GetClientQueryContext(cmd)
	if err != nil {
		return err
	}
	keyName := args[0]

	// Load configuration
	cfg, err := config.LoadConfig(clientCtx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Setup logger
	logger, err := log.NewRootLoggerWithFile(config.LogFile(clientCtx.HomeDir), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger: %w", err)
	}

	// Get database backend
	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	// Create EOTS manager
	eotsManager, err := eotsmanager.NewLocalEOTSManager(clientCtx.HomeDir, clientCtx.Keyring.Backend(), dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	// Get the public key for the newly added key
	eotsPk, err := eotsManager.LoadBIP340PubKeyFromKeyName(keyName)
	if err != nil {
		return fmt.Errorf("failed to get public key for key %s: %w", keyName, err)
	}

	// Save the public key to key name mapping
	if err := eotsManager.SaveEOTSKeyName(eotsPk.MustToBTCPK(), keyName); err != nil {
		return fmt.Errorf("failed to save key name mapping: %w", err)
	}

	k, err := clientCtx.Keyring.Key(keyName)
	if err != nil {
		return fmt.Errorf("failed to get public get key %s: %w", keyName, err)
	}

	ctx := cmd.Context()
	mnemonic := ctx.Value(mnemonicCtxKey).(string) // nolint: forcetypeassert
	showMnemonic := ctx.Value(mnemonicShowCtxKey).(bool)
	return printCreatePubKeyHex(cmd, k, eotsPk, showMnemonic, mnemonic, clientCtx.OutputFormat)
}

func printCreatePubKeyHex(cmd *cobra.Command, k *cryptokeyring.Record, eotsPk *types.BIP340PubKey, showMnemonic bool, mnemonic, outputFormat string) error {
	out, err := keys.MkAccKeyOutput(k)
	if err != nil {
		return err
	}
	keyOutput := newKeyOutputWithPubKeyHex(out, eotsPk)

	switch outputFormat {
	case flags.OutputFormatText:
		cmd.PrintErrln()
		if err := printKeyringRecord(cmd.OutOrStdout(), keyOutput, outputFormat); err != nil {
			return err
		}

		// print mnemonic unless requested not to.
		if showMnemonic {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "\n**Important** write this mnemonic phrase in a safe place.\nIt is the only way to recover your account if you ever forget your password.\n\n%s\n", mnemonic); err != nil {
				return fmt.Errorf("failed to print mnemonic: %s", err.Error())
			}
		}
	case flags.OutputFormatJSON:
		if showMnemonic {
			keyOutput.Mnemonic = mnemonic
		}

		jsonString, err := json.MarshalIndent(keyOutput, "", "  ")
		if err != nil {
			return err
		}

		cmd.Println(string(jsonString))

	default:
		return fmt.Errorf("invalid output format %s", outputFormat)
	}

	return nil
}

func newKeyOutputWithPubKeyHex(k keys.KeyOutput, eotsPk *types.BIP340PubKey) KeyOutputWithPubKeyHex {
	return KeyOutputWithPubKeyHex{
		KeyOutput: k,
		PubKeyHex: eotsPk.MarshalHex(),
	}
}

func printKeyringRecord(w io.Writer, ko KeyOutputWithPubKeyHex, output string) error {
	switch output {
	case flags.OutputFormatText:
		if err := printTextRecords(w, []KeyOutputWithPubKeyHex{ko}); err != nil {
			return err
		}

	case flags.OutputFormatJSON:
		out, err := json.Marshal(ko)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintln(w, string(out)); err != nil {
			return err
		}
	}

	return nil
}

func printTextRecords(w io.Writer, kos []KeyOutputWithPubKeyHex) error {
	out, err := yaml.Marshal(&kos)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, string(out)); err != nil {
		return err
	}

	return nil
}
