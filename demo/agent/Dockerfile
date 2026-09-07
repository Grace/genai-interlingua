FROM python:3.12-slim

WORKDIR /app

# httpx is a real dependency of traceloop-sdk that traceloop-sdk does not
# declare; without it the import fails at runtime rather than at install.
RUN pip install --no-cache-dir \
      "traceloop-sdk" \
      "openai" \
      "httpx"

COPY agent.py .

RUN useradd --uid 10002 --no-create-home agent
USER 10002

CMD ["python", "-u", "agent.py"]
