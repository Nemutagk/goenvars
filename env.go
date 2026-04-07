package goenvars

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/joho/godotenv"
)

var loadEnvOnce sync.Once
var mu sync.RWMutex
var awsSecrets []map[string]any
var awsSecretsLoaded map[string]string

func loadVars() {
	loadEnvOnce.Do(func() {
		if err := godotenv.Load(); err != nil {
			if os.IsNotExist(err) {
				log.Printf("Warning: archivo .env no encontrado (%v)", err)
			}

			log.Printf("Error cargando .env: %v", err)
		}
	})
}

func LoadAwsSecret(secretName, region string) error {
	if awsSecretsLoaded == nil {
		awsSecretsLoaded = make(map[string]string)
	}

	if awsSecrets == nil {
		awsSecrets = []map[string]any{} // Inicializar como slice vacío
	}

	if secretName == "" || region == "" {
		fmt.Println("secret_name o aws_region no definidos para cargar secretos")
		return errors.New("secret_name o aws_region no definidos para cargar secretos")
	}

	mu.Lock()
	defer mu.Unlock()

	_, ok := awsSecretsLoaded[secretName]
	if ok {
		fmt.Printf("secreto %s ya cargado, omitiendo\n", secretName)
		return nil
	}

	awsCfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		fmt.Printf("cargando configuración AWS: %v\n", err)
		return err
	}

	svc := secretsmanager.NewFromConfig(awsCfg)
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"),
	}

	result, err := svc.GetSecretValue(context.TODO(), input)
	if err != nil {
		fmt.Printf("obteniendo secreto: %v\n", err)
		return err
	}

	if result.SecretString == nil {
		fmt.Printf("SecretString vacío para %s\n", secretName)
		return fmt.Errorf("SecretString vacío para %s", secretName)
	}

	var awsSecretTmp map[string]any
	if err := json.Unmarshal([]byte(*result.SecretString), &awsSecretTmp); err != nil {
		fmt.Printf("unmarshal secreto: %v\n", err)
		return fmt.Errorf("unmarshal secreto: %w", err)
	}

	awsSecrets = append(awsSecrets, awsSecretTmp)
	awsSecretsLoaded[secretName] = secretName

	return nil
}

func LoadEnvVars() error {
	var retErr error
	loadVars()

	awsSecret := os.Getenv("AWS_SECRET_NAME")
	awsRegion := os.Getenv("AWS_REGION")
	if awsSecret != "" && awsRegion != "" {
		if err := LoadAwsSecret(awsSecret, awsRegion); err != nil {
			retErr = err
		}
	}

	return retErr
}

func GetEnv(key string, defaultValue string) string {
	LoadEnvVars()

	if v := os.Getenv(key); v != "" {
		return v
	}
	awsValue := getAwsValue(key)
	if awsValue != "" {
		awsValueStr := fmt.Sprintf("%v", awsValue)
		return awsValueStr
	}
	return defaultValue
}

func GetEnvBool(key string, defaultValue bool) bool {
	LoadEnvVars()

	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	awsValue := getAwsValue(key)
	if awsValue != "" {
		awsValueStr := fmt.Sprintf("%v", awsValue)
		if b, err := strconv.ParseBool(awsValueStr); err == nil {
			return b
		}
	}

	return defaultValue
}

func GetEnvInt(key string, defaultValue int) int {
	LoadEnvVars()

	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	awsValue := getAwsValue(key)
	if awsValue != "" {
		awsValueStr := fmt.Sprintf("%v", awsValue)
		if n, err := strconv.Atoi(awsValueStr); err == nil {
			return n
		}
	}
	return defaultValue
}

func GetEnvFloat(key string, defaultValue float64) float64 {
	LoadEnvVars()

	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	awsValue := getAwsValue(key)
	if awsValue != "" {
		awsValueStr := fmt.Sprintf("%v", awsValue)
		if f, err := strconv.ParseFloat(awsValueStr, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getAwsValue(key string) any {
	mu.RLock()
	defer mu.RUnlock()

	if awsSecrets == nil {
		return ""
	}

	for _, awsSecret := range awsSecrets {
		if awsValue, ok := awsSecret[key]; ok {
			return awsValue
		}
	}
	return ""
}
