Сгенерируйте ключи:

bash
go run cmd/genkey/main.go -private private.pem -public public.pem
Запустите сервер с приватным ключом:

bash
# через флаг
./server -crypto-key private.pem

# или через переменную окружения
export CRYPTO_KEY=private.pem
./server
Запустите агент с публичным ключом:

bash
# через флаг
./agent -crypto-key public.pem

# или через переменную окружения
export CRYPTO_KEY=public.pem
./agent