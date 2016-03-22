EXECUTABLE=golandy-server
LOG_FILE=/var/log/${EXECUTABLE}.log
GOFMT=gofmt -w
GODEPS=go get

GOFILES=\
	main.go\

build:
	go build -o ${EXECUTABLE}

install:
	go install

format:
	${GOFMT} main.go

test:

deps:
	${GODEPS} github.com/pborman/uuid
	${GODEPS} github.com/gin-gonic/gin
	${GODEPS} github.com/gorilla/websocket

stop:
	pkill -f ${EXECUTABLE}

start:
	-make stop
	cd ${GOPATH}/src/bitbucket.org/prsolucoes/golandy-server
	nohup ${EXECUTABLE} >> ${LOG_FILE} 2>&1 </dev/null &

update:
	git pull origin master
	make install
