package throttlerecorder

import (
	"os"
)

type Config struct {
	Disabled       bool
	FillAllMinutes bool

	InfluxDBURL    string
	InfluxDBToken  string
	InfluxDBOrg    string
	InfluxDBBucket string

	BigQueryProjectID string
	BigQueryDataset   string
	BigQueryTable     string
}

func LoadConfig() *Config {
	cfg := &Config{
		Disabled:       os.Getenv("THROTTLE_RESULTS_DISABLED") == "true",
		FillAllMinutes: os.Getenv("THROTTLE_RESULTS_FILL_ALL_MINUTES") == "true",

		InfluxDBURL:    getEnvOrDefault("INFLUXDB_URL", "http://localhost:8086"),
		InfluxDBToken:  os.Getenv("INFLUXDB_TOKEN"),
		InfluxDBOrg:    os.Getenv("INFLUXDB_ORG"),
		InfluxDBBucket: getEnvOrDefault("INFLUXDB_BUCKET", "throttle_results"),

		BigQueryProjectID: getEnvOrDefault("BIGQUERY_PROJECT_ID", os.Getenv("GOOGLE_CLOUD_PROJECT")),
		BigQueryDataset:   getEnvOrDefault("BIGQUERY_DATASET", "throttle_results"),
		BigQueryTable:     getEnvOrDefault("BIGQUERY_TABLE", "throttle_results"),
	}

	return cfg
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
