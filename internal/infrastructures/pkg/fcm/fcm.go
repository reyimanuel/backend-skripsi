package fcm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

var (
	once    sync.Once
	client  *messaging.Client
	initErr error
)

func Init(ctx context.Context, credentialsPath string) (*messaging.Client, error) {
	once.Do(func() {
		// Guardrail: a common mistake is putting Firebase Web config JSON here.
		// Admin SDK requires a service-account JSON (type=service_account).
		if b, err := os.ReadFile(credentialsPath); err == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				_, hasAPIKey := m["apiKey"]
				_, hasAuthDomain := m["authDomain"]
				_, hasMessagingSenderID := m["messagingSenderId"]
				_, hasAppID := m["appId"]
				_, hasClientEmail := m["client_email"]
				_, hasPrivateKey := m["private_key"]
				_, hasType := m["type"]

				looksLikeWebConfig := (hasAPIKey || hasAuthDomain || hasMessagingSenderID || hasAppID) && !(hasClientEmail || hasPrivateKey || hasType)
				if looksLikeWebConfig {
					initErr = fmt.Errorf("invalid FIREBASE_CREDENTIALS: looks like Firebase Web config (apiKey/authDomain/...) not a service-account JSON; download service account key from Firebase Console > Project Settings > Service accounts")
					return
				}
			}
		}

		// Some environments/credential files don't expose the project ID in a way the SDK can infer.
		// Allow overriding via env vars.
		var cfg *firebase.Config
		if projectID := os.Getenv("FIREBASE_PROJECT_ID"); projectID != "" {
			cfg = &firebase.Config{ProjectID: projectID}
		} else if projectID := os.Getenv("GOOGLE_CLOUD_PROJECT"); projectID != "" {
			cfg = &firebase.Config{ProjectID: projectID}
		} else if projectID := os.Getenv("GCLOUD_PROJECT"); projectID != "" {
			cfg = &firebase.Config{ProjectID: projectID}
		}

		app, err := firebase.NewApp(ctx, cfg, option.WithCredentialsFile(credentialsPath))
		if err != nil {
			initErr = err
			return
		}
		client, initErr = app.Messaging(ctx)
	})
	return client, initErr
}

func Client() *messaging.Client { return client }
