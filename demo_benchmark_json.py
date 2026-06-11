import sys
import struct
import json

def main():
    tick_format = '<I4xQdI4x'
    
    with open("ticks.bin", "rb") as f:
        data = f.read()
        
    for unpacked in struct.iter_unpack(tick_format, data):
        if unpacked[3] > 9900:
            record = {
                "symbol": unpacked[0],
                "ts": unpacked[1],
                "price": unpacked[2],
                "size": unpacked[3]
            }
            print(json.dumps(record))

if __name__ == "__main__":
    main()
