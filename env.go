package goenvars

import (
	"context"
	"encoding/json"
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

var loadAllVarsOnce sync.Once
var loadEnvOnce sync.Once
var loadAwsSecretsOnce sync.Once
var awsSecrets map[string]interface{}

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

func loadAwsSecrets() (map[string]interface{}, error) {
	var loadErr error
	loadAwsSecretsOnce.Do(func() {
		log.Println("Loading AWS secrets...")
		secretName := os.Getenv("AWS_SECRET_NAME")
		region := os.Getenv("AWS_REGION")

		if secretName == "" {
			// No definido => no se cargan secretos
			return
		}
		if region == "" {
			loadErr = fmt.Errorf("AWS_REGION no definido para cargar secretos")
			return
		}

		awsCfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		if err != nil {
			loadErr = fmt.Errorf("cargando configuración AWS: %w", err)
			return
		}

		svc := secretsmanager.NewFromConfig(awsCfg)
		input := &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(secretName),
			VersionStage: aws.String("AWSCURRENT"),
		}

		result, err := svc.GetSecretValue(context.TODO(), input)
		if err != nil {
			loadErr = fmt.Errorf("obteniendo secreto: %w", err)
			return
		}

		if result.SecretString == nil {
			loadErr = fmt.Errorf("SecretString vacío para %s", secretName)
			return
		}

		if err := json.Unmarshal([]byte(*result.SecretString), &awsSecrets); err != nil {
			loadErr = fmt.Errorf("unmarshal secreto: %w", err)
			awsSecrets = nil
			return
		}
	})
	return awsSecrets, loadErr
}

func LoadEnvVars() (bool, error) {
	var retErr error
	loadAllVarsOnce.Do(func() {
		loadVars()

		if os.Getenv("AWS_SECRET_NAME") != "" {
			if _, err := loadAwsSecrets(); err != nil {
				retErr = err
			}
		}
	})
	return retErr == nil, retErr
}

func GetEnv(key string, defaultValue string) string {
	LoadEnvVars()

	if v := os.Getenv(key); v != "" {
		return v
	}
	if awsSecrets != nil {
		if awsValue, ok := awsSecrets[key]; ok {
			if s, ok2 := awsValue.(string); ok2 {
				return s
			}
			// Fallback a representar como string genérico
			return fmt.Sprint(awsValue)
		}
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
	if awsSecrets != nil {
		if awsValue, ok := awsSecrets[key]; ok {
			if s, ok2 := awsValue.(string); ok2 {
				if b, err := strconv.ParseBool(s); err == nil {
					return b
				}
			}
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
	if awsSecrets != nil {
		if awsValue, ok := awsSecrets[key]; ok {
			if s, ok2 := awsValue.(string); ok2 {
				if n, err := strconv.Atoi(s); err == nil {
					return n
				}
			}
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
	if awsSecrets != nil {
		if awsValue, ok := awsSecrets[key]; ok {
			if s, ok2 := awsValue.(string); ok2 {
				if f, err := strconv.ParseFloat(s, 64); err == nil {
					return f
				}
			}
		}
	}
	return defaultValue
}