#!/bin/sh

# Check if AI_DEVELOPER_GITNESS_TOKEN is set
if [ -z "$AI_DEVELOPER_GITNESS_TOKEN" ]; then
  echo "AI_DEVELOPER_GITNESS_TOKEN is not set"

  # Wait until Gitness is up
  while true; do
    curl -s gitness:3000/ > /dev/null
    if [ $? -eq 0 ]; then
      break
    fi
    sleep 1
  done

  echo "Gitness is up"

  # Generate login data
  data=$(jq -n --arg login_identifier "$AI_DEVELOPER_GITNESS_USER" --arg password "$AI_DEVELOPER_GITNESS_PASSWORD" '{login_identifier: $login_identifier, password: $password}')

  # Attempt to login and get the login token
  login_response=$(curl -s -X 'POST' 'http://gitness:3000/api/v1/login?include_cookie=false' -H 'accept: application/json' -H 'Content-Type: application/json' -d "$data")
  echo "Login response: $login_response"
  
  login_token=$(echo "$login_response" | jq -r '.access_token')
  
  if [ "$login_token" = "null" ]; then
    echo "Failed to obtain login token"
    exit 1
  fi

  # Generate access token data
  current_date=$(date +%s)
  data=$(jq -n --arg identifier "$current_date" '{identifier: $identifier, lifetime: 604800000000000}')

  # Attempt to get the access token
  access_token_response=$(curl -s -X 'POST' 'http://gitness:3000/api/v1/user/tokens' -H 'accept: application/json' -H 'Content-Type: application/json' -H "Cookie: token=$login_token" -d "$data")
  echo "Access token response: $access_token_response"

  access_token=$(echo "$access_token_response" | jq -r '.access_token')
  
  if [ "$access_token" = "null" ]; then
    echo "Failed to obtain access token"
    exit 1
  fi

  export AI_DEVELOPER_GITNESS_TOKEN=$access_token
  echo "Access token obtained and exported"
else
  echo "AI_DEVELOPER_GITNESS_TOKEN is set"
fi

# Run the Go server
go run server.go
