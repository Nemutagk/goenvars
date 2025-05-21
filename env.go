package goenvars

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/joho/godotenv"
)

var loadAllVarsOnce sync.Once
var loadEnvOnce sync.Once
var loadAwsSecretsOnce sync.Once
var awsSecrets map[string]interface{}

func loadVars() {
	loadEnvOnce.Do(func() {
		fmt.Println("Loading environment variables...")
		if err := godotenv.Load(); err != nil {
			fmt.Println("Error loading .env file: ", err)
		}
	})
}

func loadAwsSecrets() (map[string]interface{}, error) {
	fmt.Println("Loading AWS secrets...")
	loadAwsSecretsOnce.Do(func() {
		secretName := GetEnv("AWS_SECRET_NAME", "")
		region := GetEnv("AWS_REGION", "")

		if secretName == "" || region == "" {
			fmt.Println("AWS_SECRET_NAME or AWS_REGION not set in environment variables")
			awsSecrets = nil
			return
		}

		fmt.Println("create config with region: ", region)
		config, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		if err != nil {
			fmt.Println("Error loading AWS config: ", err)
			return
		}

		fmt.Println("create secrets manager client")
		svc := secretsmanager.NewFromConfig(config)
		input := &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(secretName),
			VersionStage: aws.String("AWSCURRENT"),
		}

		result, err := svc.GetSecretValue(context.TODO(), input)
		if err != nil {
			fmt.Println("Error getting secret value: ", err)
			awsSecrets = nil
			return
		}

		fmt.Println("Secret value: ", awsSecrets)

		err = json.Unmarshal([]byte(*result.SecretString), &awsSecrets)

		if err != nil {
			fmt.Println("Error unmarshalling secret value: ", err)
			awsSecrets = nil
			return
		}
	})

	return awsSecrets, nil
}

func LoadEnvVars() (bool, error) {
	loadAllVarsOnce.Do(func() {
		fmt.Println("Loading environment variables...")
		app_env := os.Getenv("APP_ENV")
		if app_env == "" {
			app_env = "local"
		}

		fmt.Println("APP_ENV: ", app_env)

		if app_env != "local" {
			if _, err := loadAwsSecrets(); err != nil {
				fmt.Println("Error loading AWS secrets: ", err)
				return
			}
		} else {
			loadVars()
		}
	})

	return true, nil
}

func GetEnv(key string, defaultValue string) string {
	LoadEnvVars()

	value := os.Getenv(key)

	if value != "" {
		return value
	} else if awsSecrets != nil {
		if awsValue, ok := awsSecrets[key]; ok {
			return awsValue.(string)
		}
	}

	return defaultValue
}
