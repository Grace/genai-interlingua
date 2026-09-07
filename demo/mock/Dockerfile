# The same mock server the capture harness uses, built from its one copy in
# testdata/capture rather than a duplicate here: two copies of a file that must
# answer identically is two copies that will eventually not.
#
# Build context is the repository root.
FROM python:3.12-slim
WORKDIR /app
COPY testdata/capture/mockopenai.py .
RUN useradd --uid 10003 --no-create-home mock
USER 10003
EXPOSE 8080
ENV MOCK_HOST=0.0.0.0
CMD ["python", "-u", "mockopenai.py", "8080"]
