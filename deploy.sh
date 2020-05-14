#!/bin/bash

# Check that all changes are commited before going further.
if ! git diff --quiet; then
  echo "Commit all changes before deploying. Exiting script..."
  exit 1
fi

GOOS=linux GOARCH=amd64 go build -o server -ldflags "-X main.gitCommit=$(git rev-parse HEAD)" 'github.com/sbadame/scopa/cmd/...' || exit 2

gcloud compute instances add-metadata --zone "us-east1-b" "scopaserver" --project "scopa-273021" --metadata startup-script='
#!/bin/bash

# Configure port forwarding so that the server need not run as root.
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 80 -j REDIRECT --to-port 8080
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 443 -j REDIRECT --to-port 8081

# Run the server with dropped permissions.
cd /home/sandro/scopa
sudo -u sandro /home/sandro/scopa/server -http_port=8080 -https_port=8081 -https_host=scopa.sandr.io -random
'

gcloud compute instances start --zone "us-east1-b" "scopaserver" --project "scopa-273021"
gcloud beta compute ssh --zone "us-east1-b" "scopaserver" --project "scopa-273021" \
  --command 'mkdir -p scopa; rm scopa/server'
gcloud compute scp --zone "us-east1-b" --project "scopa-273021" --recurse server web 'scopaserver:~/scopa/'

echo "Deployed $(git rev-parse HEAD)"
