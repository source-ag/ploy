package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/source-ag/ploy/utils"
)

type EcsDeployment struct {
	BaseDeploymentConfig    `mapstructure:",squash"`
	Cluster                 string `mapstructure:"cluster"`
	VersionEnvironmentKey   string `mapstructure:"version-environment-key,omitempty"`
	WaitForServiceStability bool   `mapstructure:"wait-for-service-stability,omitempty"`
	WaitForMinutes          int    `mapstructure:"wait-for-minutes,omitempty"`
	ForceNewDeployment      bool   `mapstructure:"force-new-deployment,omitempty"`
}

type ECSDeploymentEngine struct {
	ECSClient *ecs.Client
}

func (engine *ECSDeploymentEngine) Type() string {
	return "ecs"
}

func (engine *ECSDeploymentEngine) ResolveConfigStruct() Deployment {
	return &EcsDeployment{
		ForceNewDeployment:      false,
		WaitForMinutes:          30,
		WaitForServiceStability: false,
	}
}

func (engine *ECSDeploymentEngine) Deploy(config Deployment, p func(string, ...any)) error {
	ecsConfig := config.(*EcsDeployment)
	describeServicesInput := &ecs.DescribeServicesInput{
		Services: []string{ecsConfig.Id()},
		Cluster:  aws.String(ecsConfig.Cluster),
	}
	services, err := engine.ECSClient.DescribeServices(
		context.Background(),
		describeServicesInput,
	)
	if err != nil {
		return err
	}
	runningDeployment, err := findRunningDeployment(services, ecsConfig)
	if err != nil {
		return err
	}
	taskDefinitionArn := runningDeployment.TaskDefinition
	taskDefinitionOutput, err := engine.ECSClient.DescribeTaskDefinition(context.Background(), &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: taskDefinitionArn,
	})
	p("Registering new task definition for '%s' with version '%s'...", *taskDefinitionOutput.TaskDefinition.Family, ecsConfig.Version())
	registerTaskDefinitionOutput, err := engine.ECSClient.RegisterTaskDefinition(context.Background(), generateRegisterTaskDefinitionInput(taskDefinitionOutput.TaskDefinition, ecsConfig.Version(), ecsConfig.VersionEnvironmentKey))
	if err != nil {
		return err
	}
	p("Updating service '%s' with new task definition '%s'...", ecsConfig.Id(), *registerTaskDefinitionOutput.TaskDefinition.TaskDefinitionArn)
	_, err = engine.ECSClient.UpdateService(context.Background(), &ecs.UpdateServiceInput{
		Service:            aws.String(ecsConfig.Id()),
		Cluster:            aws.String(ecsConfig.Cluster),
		TaskDefinition:     registerTaskDefinitionOutput.TaskDefinition.TaskDefinitionArn,
		ForceNewDeployment: ecsConfig.ForceNewDeployment,
	})
	if ecsConfig.WaitForServiceStability {
		p("Waiting for service '%s' to stabilize...", ecsConfig.Id())
		waitDuration := time.Duration(ecsConfig.WaitForMinutes) * time.Minute
		waiterClient := ecs.NewServicesStableWaiter(engine.ECSClient)
		err = waiterClient.Wait(context.Background(), describeServicesInput, waitDuration)
		if err != nil {
			return err
		}
		p("Service '%s' is stable", ecsConfig.Id())
	}
	if err != nil {
		return err
	}
	return nil
}

func generateRegisterTaskDefinitionInput(taskDefinition *types.TaskDefinition, version string, versionEnvironmentKey string) *ecs.RegisterTaskDefinitionInput {
	containerDefinitions := taskDefinition.ContainerDefinitions
	for i := range containerDefinitions {
		imageName := strings.Split(*containerDefinitions[i].Image, ":")[0]
		containerDefinitions[i].Image = aws.String(imageName + ":" + version)
		if versionEnvironmentKey != "" {
			containerDefinitions[i].Environment = append(containerDefinitions[i].Environment, types.KeyValuePair{
				Name:  aws.String(versionEnvironmentKey),
				Value: aws.String(version),
			})
		}
	}
	registerTaskDefinitionInput := &ecs.RegisterTaskDefinitionInput{
		Family:                  taskDefinition.Family,
		ContainerDefinitions:    containerDefinitions,
		Cpu:                     taskDefinition.Cpu,
		EphemeralStorage:        taskDefinition.EphemeralStorage,
		ExecutionRoleArn:        taskDefinition.ExecutionRoleArn,
		InferenceAccelerators:   taskDefinition.InferenceAccelerators,
		IpcMode:                 taskDefinition.IpcMode,
		Memory:                  taskDefinition.Memory,
		NetworkMode:             taskDefinition.NetworkMode,
		PidMode:                 taskDefinition.PidMode,
		PlacementConstraints:    taskDefinition.PlacementConstraints,
		ProxyConfiguration:      taskDefinition.ProxyConfiguration,
		RequiresCompatibilities: taskDefinition.RequiresCompatibilities,
		RuntimePlatform:         taskDefinition.RuntimePlatform,
		TaskRoleArn:             taskDefinition.TaskRoleArn,
		Volumes:                 taskDefinition.Volumes,
	}
	return registerTaskDefinitionInput
}

// TODO: deal with task definitions without a service (i.e. one-off tasks). Maybe separate out into a separate engine?

func (engine *ECSDeploymentEngine) CheckVersion(config Deployment) (string, error) {
	ecsConfig := config.(*EcsDeployment)
	services, err := engine.ECSClient.DescribeServices(
		context.Background(),
		&ecs.DescribeServicesInput{
			Services: []string{ecsConfig.Id()},
			Cluster:  aws.String(ecsConfig.Cluster),
		},
	)
	if err != nil {
		return "", err
	}
	runningDeployment, err := findRunningDeployment(services, ecsConfig)
	if err != nil {
		return "", err
	}
	taskDefinitionArn := runningDeployment.TaskDefinition
	taskDefinitionOutput, err := engine.ECSClient.DescribeTaskDefinition(context.Background(), &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: taskDefinitionArn,
	})
	if err != nil {
		return "", err
	}
	return strings.Split(*taskDefinitionOutput.TaskDefinition.ContainerDefinitions[0].Image, ":")[1], nil
}

func findRunningDeployment(services *ecs.DescribeServicesOutput, deploymentConfig *EcsDeployment) (*types.Deployment, error) {
	if len(services.Services) < 1 {
		return nil, fmt.Errorf("service %s does not exist", deploymentConfig.Id())
	}
	deployments := services.Services[0].Deployments
	runningDeployment := utils.Find(deployments, func(d types.Deployment) bool {
		return *d.Status == "PRIMARY" && d.DesiredCount == d.RunningCount
	})
	if runningDeployment == nil {
		runningDeployment = utils.Find(deployments, func(d types.Deployment) bool {
			return *d.Status == "ACTIVE" && d.DesiredCount == d.RunningCount
		})
	}
	if runningDeployment == nil {
		return nil, fmt.Errorf("no running deployment found for %s", deploymentConfig.Id())
	}
	return runningDeployment, nil
}

func init() {
	RegisterDeploymentEngine("ecs", func(awsConfig aws.Config) DeploymentEngine {
		return &ECSDeploymentEngine{ECSClient: ecs.NewFromConfig(awsConfig)}
	})
}
