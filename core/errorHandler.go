package blockchain

import (
	log "github.com/sirupsen/logrus"
)

// Handle 错误处理，这里比较简单，有待优化
func Handle(err error) {

	if err != nil {
		log.Panic(err)
	}
}
