package goenvars

import (
	"context"
	"encoding/json"
	"log"
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
		log.Println("Loading environment variables...")
		if err := godotenv.Load(); err != nil {
			log.Println("Error loading .env file: ", err)
		}
	})
}

func loadAwsSecrets() (map[string]interface{}, error) {
	log.Println("Loading AWS secrets...")
	loadAwsSecretsOnce.Do(func() {
		secretName := os.Getenv("AWS_SECRET_NAME")
		region := os.Getenv("AWS_REGION")

		if secretName == "" || region == "" {
			log.Println("AWS_SECRET_NAME or AWS_REGION not set in environment variables")
			awsSecrets = nil
			return
		}

		config, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		if err != nil {
			log.Println("Error loading AWS config: ", err)
			return
		}

		svc := secretsmanager.NewFromConfig(config)
		input := &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(secretName),
			VersionStage: aws.String("AWSCURRENT"),
		}

		result, err := svc.GetSecretValue(context.TODO(), input)
		if err != nil {
			log.Println("Error getting secret value: ", err)
			awsSecrets = nil
			return
		}

		err = json.Unmarshal([]byte(*result.SecretString), &awsSecrets)

		if err != nil {
			log.Println("Error unmarshalling secret value: ", err)
			awsSecrets = nil
			return
		}
	})

	return awsSecrets, nil
}

func LoadEnvVars() (bool, error) {
	loadAllVarsOnce.Do(func() {
		log.Println("Loading environment variables...")
		app_env := os.Getenv("APP_ENV")
		if app_env == "" {
			app_env = "local"
		}

		log.Println("APP_ENV: ", app_env)

		if app_env != "local" {
			if _, err := loadAwsSecrets(); err != nil {
				log.Println("Error loading AWS secrets: ", err)
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
