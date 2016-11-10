FROM centos:centos7

LABEL io.k8s.display-name="Custom Deployment strategy controller" \
      io.k8s.description="This image contains custom deployment strategy controller for Kubernetes"

ENTRYPOINT ["/usr/bin/custom-deployment", "start"]

ADD _output/bin/custom-deployment /usr/bin/custom-deployment
USER 1001
