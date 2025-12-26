FROM ghcr.io/openfaas/of-watchdog:0.9.11 as watchdog
FROM python:3.11-alpine

COPY --from=watchdog /fwatchdog /usr/bin/fwatchdog
RUN chmod +x /usr/bin/fwatchdog

# Create app directory
WORKDIR /home/app

# Copy function code
COPY handler.py .

# Set environment variables
ENV fprocess="python3 handler.py"
ENV mode="streaming"
ENV upstream_url="http://127.0.0.1:8080"

# Health check
HEALTHCHECK --interval=5s CMD [ -e /tmp/.lock ] || exit 1

# Run watchdog
CMD ["fwatchdog"]
