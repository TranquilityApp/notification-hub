GO=/usr/local/go/bin/go

echo Step 1/4: Creating build;
  $GO build

echo Step 2/4: Archiving previous image;
  now=$(date +"%m_%d_%Y");
  docker tag tranquilityonline/websocket-hub:latest tranquilityonline/websocket-hub:$now;
  docker push tranquilityonline/websocket-hub;

echo Step 3/4: Create new image;
  docker build -t tranquilityonline/websocket-hub .
  docker push tranquilityonline/websocket-hub;

echo Step 4/4: Deploying to Elastic Beanstalk;

EB=/home/miller/.local/bin/eb
$EB deploy websocket-hub-dev;
