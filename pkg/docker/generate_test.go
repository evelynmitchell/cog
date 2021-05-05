package docker

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/replicate/cog/pkg/model"
)

func installCog() string {
	cogLibB64 := base64.StdEncoding.EncodeToString(cogLibrary)
	return fmt.Sprintf(`RUN ### --> Installing Cog
RUN pip install flask requests redis
ENV PYTHONPATH=/usr/local/lib/cog
RUN mkdir -p /usr/local/lib/cog && echo %s | base64 --decode > /usr/local/lib/cog/cog.py`, cogLibB64)
}

func installPython(version string) string {
	return fmt.Sprintf(`RUN ### --> Installing Python prerequisites
ENV PATH="/root/.pyenv/shims:/root/.pyenv/bin:$PATH"
RUN apt-get update -q && apt-get install -qy --no-install-recommends \
	make \
	build-essential \
	libssl-dev \
	zlib1g-dev \
	libbz2-dev \
	libreadline-dev \
	libsqlite3-dev \
	wget \
	curl \
	llvm \
	libncurses5-dev \
	libncursesw5-dev \
	xz-utils \
	tk-dev \
	libffi-dev \
	liblzma-dev \
	python-openssl \
	git \
	ca-certificates \
	&& rm -rf /var/lib/apt/lists/*
RUN ### --> Installing Python 3.8
RUN curl https://pyenv.run | bash && \
	git clone https://github.com/momo-lab/pyenv-install-latest.git "$(pyenv root)"/plugins/pyenv-install-latest && \
	pyenv install-latest "%s" && \
	pyenv global $(pyenv install-latest --print "%s")
`, version, version)
}

func TestGenerateEmpty(t *testing.T) {
	conf, err := model.ConfigFromYAML([]byte(`
model: infer.py:Model
`))
	require.NoError(t, err)
	require.NoError(t, conf.ValidateAndCompleteConfig())

	expectedCPU := `FROM ubuntu:20.04
ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1
ENV LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/lib/x86_64-linux-gnu
` + installPython("3.8") + installCog() + `
RUN ### --> Copying code
COPY . /code
` + helperScripts() + `
WORKDIR /code
CMD /usr/bin/cog-http-server`

	expectedGPU := `FROM nvidia/cuda:11.0-cudnn8-devel-ubuntu16.04
ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1
ENV LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/lib/x86_64-linux-gnu
` + installPython("3.8") + installCog() + `
RUN ### --> Copying code
COPY . /code
` + helperScripts() + `
WORKDIR /code
CMD /usr/bin/cog-http-server`

	gen := DockerfileGenerator{Config: conf, Arch: "cpu"}
	actualCPU, err := gen.Generate()
	require.NoError(t, err)
	gen = DockerfileGenerator{Config: conf, Arch: "gpu"}
	actualGPU, err := gen.Generate()
	require.NoError(t, err)

	require.Equal(t, expectedCPU, actualCPU)
	require.Equal(t, expectedGPU, actualGPU)
}

func TestGenerateFull(t *testing.T) {
	conf, err := model.ConfigFromYAML([]byte(`
environment:
  python_requirements: my-requirements.txt
  python_packages:
    - torch==1.5.1
    - pandas==1.2.0.12
  system_packages:
    - ffmpeg
    - cowsay
model: infer.py:Model
`))
	require.NoError(t, err)
	require.NoError(t, conf.ValidateAndCompleteConfig())

	expectedCPU := `FROM ubuntu:20.04
ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1
ENV LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/lib/x86_64-linux-gnu
` + installPython("3.8") + `RUN ### --> Installing system packages
RUN apt-get update -qq && apt-get install -qy ffmpeg cowsay && rm -rf /var/lib/apt/lists/*
RUN ### --> Installing Python requirements
COPY my-requirements.txt /tmp/requirements.txt
RUN pip install -r /tmp/requirements.txt && rm /tmp/requirements.txt
RUN ### --> Installing Python packages
RUN pip install -f https://download.pytorch.org/whl/torch_stable.html   torch==1.5.1+cpu pandas==1.2.0.12
` + installCog() + `
RUN ### --> Copying code
COPY . /code
` + helperScripts() + `
WORKDIR /code
CMD /usr/bin/cog-http-server`

	expectedGPU := `FROM nvidia/cuda:10.2-cudnn8-devel-ubuntu18.04
ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1
ENV LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/lib/x86_64-linux-gnu
` + installPython("3.8") + `RUN ### --> Installing system packages
RUN apt-get update -qq && apt-get install -qy ffmpeg cowsay && rm -rf /var/lib/apt/lists/*
RUN ### --> Installing Python requirements
COPY my-requirements.txt /tmp/requirements.txt
RUN pip install -r /tmp/requirements.txt && rm /tmp/requirements.txt
RUN ### --> Installing Python packages
RUN pip install   torch==1.5.1 pandas==1.2.0.12
` + installCog() + `
RUN ### --> Copying code
COPY . /code
` + helperScripts() + `
WORKDIR /code
CMD /usr/bin/cog-http-server`

	gen := DockerfileGenerator{Config: conf, Arch: "cpu"}
	actualCPU, err := gen.Generate()
	require.NoError(t, err)
	gen = DockerfileGenerator{Config: conf, Arch: "gpu"}
	actualGPU, err := gen.Generate()
	require.NoError(t, err)

	require.Equal(t, expectedCPU, actualCPU)
	require.Equal(t, expectedGPU, actualGPU)
}

func helperScripts() string {
	return `
RUN echo '#!/usr/bin/env python\nimport sys\nimport cog\nimport os\nos.chdir("/code")\nsys.path.append("/code")\nfrom infer import Model\ncog.HTTPServer(Model()).start_server()' > /usr/bin/cog-http-server
RUN chmod +x /usr/bin/cog-http-server
RUN echo '#!/usr/bin/env python\nimport sys\nimport cog\nimport os\nos.chdir("/code")\nsys.path.append("/code")\nfrom infer import Model\ncog.AIPlatformPredictionServer(Model()).start_server()' > /usr/bin/cog-ai-platform-prediction-server
RUN chmod +x /usr/bin/cog-ai-platform-prediction-server
RUN echo '#!/usr/bin/env python\nimport sys\nimport cog\nimport os\nos.chdir("/code")\nsys.path.append("/code")\nfrom infer import Model\ncog.RedisQueueWorker(Model(), redis_host=sys.argv[1], redis_port=sys.argv[2], input_queue=sys.argv[3], upload_url=sys.argv[4], consumer_id=sys.argv[5]).start()' > /usr/bin/cog-redis-queue-worker
RUN chmod +x /usr/bin/cog-redis-queue-worker`
}