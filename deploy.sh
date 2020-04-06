#!/bin/bash

gcloud beta compute ssh --zone "us-east1-b" "scopaserver" --project "scopa-273021" --command 'mkdir -p scopa'
gcloud compute scp --zone "us-east1-b" --project "scopa-273021" --recurse server web 'scopaserver:~/scopa/'
