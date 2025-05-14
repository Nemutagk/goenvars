package goenvars

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/joho/godotenv"
)

var loadEnvOnce sync.Once
var loadAwsSecretsOnce sync.Once
var awsSecrets string

func loadVars() {
	loadEnvOnce.Do(func() {
		fmt.Println("Loading environment variables...")
		if err := godotenv.Overload(); err != nil {
			fmt.Println("Error loading .env file: ", err)
		}
	})
}

func loadAwsSecrets() string {
	loadAwsSecretsOnce.Do(func() {
		secretName := GetEnv("AWS_SECRET_NAME", "")
		region := GetEnv("AWS_REGION", "")

		config, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		if err != nil {
			fmt.Println("Error loading AWS config: ", err)
			return
		}

		svc := secretsmanager.NewFromConfig(config)
		input := &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(secretName),
			VersionStage: aws.String("AWSCURRENT"),
		}

		result, err := svc.GetSecretValue(context.TODO(), input)
		if err != nil {
			fmt.Println("Error getting secret value: ", err)
			return
		}

		awsSecrets = *result.SecretString
		fmt.Println("Secret value: ", awsSecrets)
	})

	return awsSecrets
}

func GetEnv(key string, defaultValue string) string {
	app_env := os.Getenv("APP_ENV")
	if app_env == "" {
		app_env = "local"
	}

	if app_env != "local" {
		loadAwsSecrets()
	} else {
		loadVars()
	}

	value := os.Getenv(key)

	if value != "" {
		return value
	}

	return defaultValue
}
