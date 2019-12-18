# Connecting to AWS

When connecting to ElastiCache redis on AWS, this EC2 instance must be in the
same subnet as the ElastiCache instance.

# Deployment
The build.sh file is the deploy script. Use it to build the application,
archive previous Docker image, build the new Docker image, push the new 
Docker image to Docker Hub, and finally deploy.
