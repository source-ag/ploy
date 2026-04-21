package deployments

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/source-ag/ploy/engine"
	"github.com/spf13/cobra"
)

func Deploy(_ *cobra.Command, args []string) {
	cfg, err := LoadConfigFromFile(args[0])
	cobra.CheckErr(err)

	errorChan := make(chan error, len(cfg.Deployments))
	var wg sync.WaitGroup
	for _, deployment := range cfg.Deployments {
		wg.Add(1)
		go func(deployment engine.Deployment) {
			defer wg.Done()
			errorChan <- doDeployment(deployment, cfg.Settings.PostDeployCommands)
		}(deployment)
	}
	wg.Wait()
	close(errorChan)
	var result *multierror.Error
	for err := range errorChan {
		result = multierror.Append(result, err)
	}
	cobra.CheckErr(result.ErrorOrNil())
}

func doDeployment(deploymentConfig engine.Deployment, globalPostDeployCommands [][]string) error {
	p := CreateDeploymentPrinter(deploymentConfig.Id())
	deploymentEngine, err := engine.GetEngine(deploymentConfig.Type())
	if err != nil {
		return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
	}
	p("checking deployed version...")
	version, err := deploymentEngine.CheckVersion(deploymentConfig)
	if err != nil {
		return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
	}
	if version != deploymentConfig.Version() {
		p("version '%s' does not match expected version '%s'. Deploying new version...", version, deploymentConfig.Version())
		if len(deploymentConfig.PreDeployCommand()) > 0 {
			if err = runDeploymentScript("pre", deploymentConfig.PreDeployCommand(), deploymentConfig.Version(), deploymentConfig.Id(), p); err != nil {
				return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
			}
		}
		if err = deploymentEngine.Deploy(deploymentConfig, p); err != nil {
			return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
		}
		runGlobalCommands(globalPostDeployCommands, deploymentConfig.Id(), deploymentConfig.Version(), p)
		if len(deploymentConfig.PostDeployCommand()) > 0 {
			if err = runDeploymentScript("post", deploymentConfig.PostDeployCommand(), deploymentConfig.Version(), deploymentConfig.Id(), p); err != nil {
				return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
			}
		}
		p("version %s deployed successfully!", deploymentConfig.Version())
		return nil
	}
	p("version '%s' matches expected version '%s'. Skipping...", version, deploymentConfig.Version())
	return nil
}

func runGlobalCommands(commands [][]string, deploymentId string, version string, p func(string, ...any)) {
	for _, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}
		if err := runDeploymentScript("global post", cmd, version, deploymentId, p); err != nil {
			p("global post-deploy command failed (ignored): %v", err)
		}
	}
}

func runDeploymentScript(context string, deploymentCommand []string, version string, deploymentId string, p func(string, ...any)) error {
	p("running %s-deployment command %s...", context, strings.Join(deploymentCommand, " "))
	cmd := exec.Command(deploymentCommand[0], deploymentCommand[1:]...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("VERSION=%s", version))
	cmd.Env = append(cmd.Env, fmt.Sprintf("DEPLOYMENT_ID=%s", deploymentId))
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err = cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	if err = cmd.Wait(); err != nil {
		return err
	}

	return nil
}
