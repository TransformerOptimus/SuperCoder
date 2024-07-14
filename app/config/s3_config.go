package config

func AWSAccessKeyID() string { return config.String("aws.access.key.id") }

func AWSSecretAccessKey() string { return config.String("aws.secret.access.key") }

func AWSBucketName() string { return config.String("aws.bucket.name") }

func AWSRegion() string { return config.String("aws.region") }
