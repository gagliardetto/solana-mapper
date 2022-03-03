# solana-mapper

## usage

```bash
go build -o mapper.bin
ulimit -n 65536 		# need lots of file descriptors
sudo su 				# need net privileges for traceroute
mkdir data 				# the dir where the results file will be saved
./mapper.bin map -c=100 --dir=data
```

NOTE: it will take a while to complete
---

## view top results

```bash
./mapper.bin view /path/to/result.json
```
