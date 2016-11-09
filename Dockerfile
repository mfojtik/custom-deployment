FROM openshift/origin

LABEL io.k8s.display-name="Custom Deployment strategy controller" \
      io.k8s.description=""

ADD _output/bin/custom-deployment /usr/bin/custom-deployment

# The observer doesn't require a root user.
USER 1001
ENTRYPOINT ["/usr/bin/custom-deployment"]
