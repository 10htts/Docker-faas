FROM ghcr.io/openfaas/of-watchdog:0.9.11 AS watchdog

FROM python:3.11-slim AS builder
WORKDIR /home/app
COPY requirements.txt ./
RUN pip install --no-cache-dir uv && \
    uv pip install --system --compile -r requirements.txt

FROM python:3.11-slim

COPY --from=watchdog /fwatchdog /usr/bin/fwatchdog
RUN chmod +x /usr/bin/fwatchdog

WORKDIR /home/app
COPY --from=builder /usr/local /usr/local
COPY handler.py pyproject.toml ./

ENV fprocess="python handler.py"
ENV mode="streaming"
ENV upstream_url="http://127.0.0.1:8080"

EXPOSE 8080
HEALTHCHECK --interval=5s CMD [ -e /tmp/.lock ] || exit 1

CMD ["fwatchdog"]

