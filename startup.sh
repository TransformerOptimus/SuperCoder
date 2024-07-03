#!/bin/sh

#check if AI_DEVELOPER_GITNESS_TOKEN is set and if not set it continue else exit
if [ -z "$AI_DEVELOPER_GITNESS_TOKEN" ]; then
  echo "AI_DEVELOPER_GITNESS_TOKEN is not set"
  while true; do
    curl -s gitness:3000/ > /dev/null
    if [ $? -eq 0 ]; then
      break
    fi
    sleep 1
  done

  echo "Gitness is up"

  data=$(jq -n --arg login_identifier "$AI_DEVELOPER_GITNESS_USER" --arg password "$AI_DEVELOPER_GITNESS_PASSWORD" '{login_identifier: $login_identifier, password: $password}')

  admin_token=$(curl -X 'POST' 'http://gitness:3000/api/v1/login?include_cookie=false' -H 'accept: application/json' -H 'Content-Type: application/json' -d "$data"  | jq -r '.access_token')

  export AI_DEVELOPER_GITNESS_TOKEN=$admin_token
else
  echo "AI_DEVELOPER_GITNESS_TOKEN is set"
fi

go run server.go