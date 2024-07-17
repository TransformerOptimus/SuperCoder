package config

func OpenSearchURL() string {
	return config.String("open.search.url")

}

func OpenSearchUsername() string {
	return config.String("open.search.username")
}

func OpenSearchPassword() string {
	return config.String("open.search.password")
}
