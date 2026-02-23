package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventbridgetypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

type EcsScheduledTaskDeployment struct {
	BaseDeploymentConfig  `mapstructure:",squash"`
	Rule                  string `mapstructure:"rule"`
	EventBusName          string `mapstructure:"event-bus-name,omitempty"`
	VersionEnvironmentKey string `mapstructure:"version-environment-key,omitempty"`
}

type ECSScheduledTaskDeploymentEngine struct {
	ECSClient         *ecs.Client
	EventBridgeClient *eventbridge.Client
}

func (engine *ECSScheduledTaskDeploymentEngine) Type() string {
	return "ecs-scheduled-task"
}

func (engine *ECSScheduledTaskDeploymentEngine) ResolveConfigStruct() Deployment {
	return &EcsScheduledTaskDeployment{}
}

func (engine *ECSScheduledTaskDeploymentEngine) Deploy(config Deployment, p func(string, ...any)) error {
	taskConfig := config.(*EcsScheduledTaskDeployment)

	if taskConfig.Rule == "" {
		return fmt.Errorf("ecs-scheduled-task '%s': 'rule' must be set", taskConfig.Id())
	}

	targets, err := engine.findMatchingTargets(taskConfig)
	if err != nil {
		return err
	}

	taskDefinitionOutput, err := engine.ECSClient.DescribeTaskDefinition(context.Background(), &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: targets[0].EcsParameters.TaskDefinitionArn,
	})
	if err != nil {
		return err
	}

	p("Registering new task definition for '%s' with version '%s'...", *taskDefinitionOutput.TaskDefinition.Family, taskConfig.Version())
	registerOutput, err := engine.ECSClient.RegisterTaskDefinition(
		context.Background(),
		generateRegisterTaskDefinitionInput(taskDefinitionOutput.TaskDefinition, taskConfig.Version(), taskConfig.VersionEnvironmentKey),
	)
	if err != nil {
		return err
	}

	newTaskDefArn := registerOutput.TaskDefinition.TaskDefinitionArn
	p("Updating %d ECS target(s) on EventBridge rule '%s' with new task definition '%s'...", len(targets), taskConfig.Rule, *newTaskDefArn)

	updatedTargets := make([]eventbridgetypes.Target, len(targets))
	for i, target := range targets {
		updated := target
		updatedEcsParams := *target.EcsParameters
		updatedEcsParams.TaskDefinitionArn = newTaskDefArn
		updated.EcsParameters = &updatedEcsParams
		updatedTargets[i] = updated
	}

	putOutput, err := engine.EventBridgeClient.PutTargets(context.Background(), &eventbridge.PutTargetsInput{
		Rule:         aws.String(taskConfig.Rule),
		EventBusName: aws.String(engine.resolveEventBusName(taskConfig)),
		Targets:      updatedTargets,
	})
	if err != nil {
		return err
	}
	if putOutput.FailedEntryCount > 0 {
		return fmt.Errorf(
			"failed to update %d target(s) on EventBridge rule '%s': %s",
			putOutput.FailedEntryCount,
			taskConfig.Rule,
			*putOutput.FailedEntries[0].ErrorMessage,
		)
	}
	return nil
}

func (engine *ECSScheduledTaskDeploymentEngine) CheckVersion(config Deployment) (string, error) {
	taskConfig := config.(*EcsScheduledTaskDeployment)

	if taskConfig.Rule == "" {
		return "", fmt.Errorf("ecs-scheduled-task '%s': 'rule' must be set", taskConfig.Id())
	}

	targets, err := engine.findMatchingTargets(taskConfig)
	if err != nil {
		return "", err
	}

	taskDefinitionOutput, err := engine.ECSClient.DescribeTaskDefinition(context.Background(), &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: targets[0].EcsParameters.TaskDefinitionArn,
	})
	if err != nil {
		return "", err
	}

	return strings.Split(*taskDefinitionOutput.TaskDefinition.ContainerDefinitions[0].Image, ":")[1], nil
}

// findMatchingTargets returns all targets on the configured rule whose task definition family
// matches the deployment id.
func (engine *ECSScheduledTaskDeploymentEngine) findMatchingTargets(taskConfig *EcsScheduledTaskDeployment) ([]eventbridgetypes.Target, error) {
	allTargets, err := engine.listAllTargets(taskConfig.Rule, engine.resolveEventBusName(taskConfig))
	if err != nil {
		return nil, err
	}

	var matching []eventbridgetypes.Target
	for _, target := range allTargets {
		if target.EcsParameters != nil &&
			target.EcsParameters.TaskDefinitionArn != nil &&
			taskDefinitionFamily(*target.EcsParameters.TaskDefinitionArn) == taskConfig.Id() {
			matching = append(matching, target)
		}
	}

	if len(matching) == 0 {
		return nil, fmt.Errorf("no targets for task definition family '%s' found on EventBridge rule '%s'", taskConfig.Id(), taskConfig.Rule)
	}

	return matching, nil
}

func (engine *ECSScheduledTaskDeploymentEngine) listAllTargets(ruleName, eventBusName string) ([]eventbridgetypes.Target, error) {
	var targets []eventbridgetypes.Target
	var nextToken *string
	for {
		output, err := engine.EventBridgeClient.ListTargetsByRule(context.Background(), &eventbridge.ListTargetsByRuleInput{
			Rule:         aws.String(ruleName),
			EventBusName: aws.String(eventBusName),
			NextToken:    nextToken,
		})
		if err != nil {
			return nil, err
		}
		targets = append(targets, output.Targets...)
		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}
	return targets, nil
}

// taskDefinitionFamily extracts the family name from a task definition ARN or "family:revision" string.
// e.g. "arn:aws:ecs:us-east-1:123456789012:task-definition/my-task:5" → "my-task"
// e.g. "my-task:5" → "my-task"
func taskDefinitionFamily(arnOrName string) string {
	s := arnOrName
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		s = s[idx+1:]
	}
	return strings.Split(s, ":")[0]
}

func (engine *ECSScheduledTaskDeploymentEngine) resolveEventBusName(taskConfig *EcsScheduledTaskDeployment) string {
	if taskConfig.EventBusName != "" {
		return taskConfig.EventBusName
	}
	return "default"
}

func init() {
	RegisterDeploymentEngine("ecs-scheduled-task", func(awsConfig aws.Config) DeploymentEngine {
		return &ECSScheduledTaskDeploymentEngine{
			ECSClient:         ecs.NewFromConfig(awsConfig),
			EventBridgeClient: eventbridge.NewFromConfig(awsConfig),
		}
	})
}
