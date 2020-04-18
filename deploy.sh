#!/bin/bash

# Check that all changes are commited before going further.
if ! git diff --quiet; then
  echo "Commit all changes before deploying. Exiting script..."
  exit 1
fi

GOOS=linux GOARCH=amd64 go build -ldflags "-X main.gitCommit=$(git rev-parse HEAD)" cmd/server.go || exit 2

gcloud compute instances add-metadata --zone "us-east1-b" "scopaserver" --project "scopa-273021" --metadata startup-script='
#!/bin/bash

# Make sure that we have setcap
sudo apt-get install libcap2-bin

cd /home/sandro/scopa

# Grant the server permission to bind to low ports.
sudo setcap cap_net_bind_service=+pie server

# Run the server with dropped permissions.
sudo -u sandro /home/sandro/scopa/server -https_host scopa.sandr.io -random
'

gcloud compute instances start --zone "us-east1-b" "scopaserver" --project "scopa-273021"
gcloud beta compute ssh --zone "us-east1-b" "scopaserver" --project "scopa-273021" \
  --command 'mkdir -p scopa; rm scopa/server'
gcloud compute scp --zone "us-east1-b" --project "scopa-273021" --recurse server web 'scopaserver:~/scopa/'
