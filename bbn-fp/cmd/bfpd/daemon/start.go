package daemon

import (
	"fmt"
	"net"
	"path/filepath"

	"github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	fpcmd "github.com/Manta-Network/manta-fp/bbn-fp/cmd"
	fpcfg "github.com/Manta-Network/manta-fp/bbn-fp/config"
	"github.com/Manta-Network/manta-fp/bbn-fp/service"
	"github.com/Manta-Network/manta-fp/log"
	"github.com/Manta-Network/manta-fp/util"
)

// CommandStart returns the start command of bfpd daemon.
func CommandStart() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "start",
		Short:   "Start the bbn-fp app daemon.",
		Long:    `Start the bbn-fp app. Note that eotsd should be started beforehand`,
		Example: `bfpd start --home /home/user/.bfpd`,
		Args:    cobra.NoArgs,
		RunE:    fpcmd.RunEWithClientCtx(runStartCmd),
	}
	cmd.Flags().String(fpEotsPkFlag, "", "The EOTS public key of the bbn-fp to start")
	cmd.Flags().String(passphraseFlag, "", "The pass phrase used to decrypt the private key")
	cmd.Flags().String(rpcListenerFlag, "", "The address that the RPC server listens to")
	return cmd
}

func runStartCmd(ctx client.Context, cmd *cobra.Command, _ []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)
	flags := cmd.Flags()

	fpStr, err := flags.GetString(fpEotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpEotsPkFlag, err)
	}

	rpcListener, err := flags.GetString(rpcListenerFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", rpcListenerFlag, err)
	}

	passphrase, err := flags.GetString(passphraseFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", passphraseFlag, err)
	}

	cfg, err := fpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if rpcListener != "" {
		_, err := net.ResolveTCPAddr("tcp", rpcListener)
		if err != nil {
			return fmt.Errorf("invalid RPC listener address %s, %w", rpcListener, err)
		}
		cfg.RPCListener = rpcListener
	}

	logger, err := log.NewRootLoggerWithFile(fpcfg.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	fpApp, err := loadApp(logger, cfg, dbBackend)
	if err != nil {
		return fmt.Errorf("failed to load app: %w", err)
	}

	if err := startApp(fpApp, fpStr, passphrase); err != nil {
		return fmt.Errorf("failed to start app: %w", err)
	}

	// Hook interceptor for os signals.
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		return err
	}

	fpServer := service.NewFinalityProviderServer(cfg, logger, fpApp, dbBackend, shutdownInterceptor)
	return fpServer.RunUntilShutdown()
}

// loadApp initialize an finality provider app based on config and flags set.
func loadApp(
	logger *zap.Logger,
	cfg *fpcfg.Config,
	dbBackend walletdb.DB,
) (*service.FinalityProviderApp, error) {
	fpApp, err := service.NewFinalityProviderAppFromConfig(cfg, dbBackend, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create bbn-fp app: %w", err)
	}

	return fpApp, nil
}

// startApp starts the app and the handle of finality providers if needed based on flags.
func startApp(
	fpApp *service.FinalityProviderApp,
	fpPkStr, passphrase string,
) error {
	// only start the app without starting any finality provider instance
	// this is needed for new finality provider registration or unjailing
	// finality providers
	if err := fpApp.Start(); err != nil {
		return fmt.Errorf("failed to start the finality provider app: %w", err)
	}

	// no fp instance will be started if public key is not specified
	if fpPkStr == "" {
		return nil
	}

	// start the bbn-fp instance with the given public key
	fpPk, err := types.NewBIP340PubKeyFromHex(fpPkStr)
	if err != nil {
		return fmt.Errorf("invalid finality provider public key %s: %w", fpPkStr, err)
	}

	if err := fpApp.StartFinalityProvider(fpPk, passphrase); err != nil {
		return fmt.Errorf("failed to start the bbn-fp instance %s: %w", fpPkStr, err)
	}

	return nil
}
