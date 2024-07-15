# Deploy: tell Tilt what YAML to deploy
k8s_yaml('app.yaml')

# Build: tell Tilt what images to build from which directories
docker_build('gedcb/example', '.')

# Watch: tell Tilt how to connect locally (optional)
k8s_resource('frontend', port_forwards=8080)