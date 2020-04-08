#!/bin/bash

go build cmd/server/server.go || exit 2

gcloud compute instances start --zone "us-east1-b" "scopaserver" --project "scopa-273021" 
gcloud beta compute ssh --zone "us-east1-b" "scopaserver" --project "scopa-273021" \
  --command 'mkdir -p scopa; rm scopa/server'
gcloud compute scp --zone "us-east1-b" --project "scopa-273021" --recurse server web 'scopaserver:~/scopa/'
