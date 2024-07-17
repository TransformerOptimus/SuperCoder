package config

import (
	"ai-developer/app/constants"
	"crypto/tls"
	"fmt"
	"github.com/opensearch-project/opensearch-go"
	"net/http"
)

func OpenSearchURL() string {
	return config.String("open.search.url")
}

func OpenSearchUsername() string {
	return config.String("open.search.username")
}

func OpenSearchPassword() string {
	return config.String("open.search.password")
}

func InitOpenSearch() (client *opensearch.Client) {
	insecureSkipVerify := AppEnv() == constants.Development
	cfg := opensearch.Config{
		Addresses: []string{
			OpenSearchURL(),
		},
		Username: OpenSearchUsername(),
		Password: OpenSearchPassword(),
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureSkipVerify,
			},
		},
	}
	openSearchClient, err := opensearch.NewClient(cfg)
	fmt.Println("Creating OpenSearch client.....")
	if err != nil {
		panic(err)
	}
	return openSearchClient
}
