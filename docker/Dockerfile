FROM golang:1.8.1
MAINTAINER Paulo Coutinho <paulo@prsolucoes.com>

ENV DEBIAN_FRONTEND noninteractive

# define timezone
RUN echo "America/Sao_Paulo" > /etc/timezone
RUN dpkg-reconfigure -f noninteractive tzdata
RUN /bin/echo -e "LANG=\"en_US.UTF-8\"" > /etc/default/local

# get project
RUN go get -u github.com/prsolucoes/golandy-server
RUN go install github.com/prsolucoes/golandy-server