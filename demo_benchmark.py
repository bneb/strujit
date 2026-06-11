import sys
import struct

def main():
    tick_format = '<I4xQdI4x'
    tick_size = struct.calcsize(tick_format)
    
    with open("ticks.bin", "rb") as f:
        data = f.read()
        
    out = sys.stdout.buffer
    for unpacked in struct.iter_unpack(tick_format, data):
        if unpacked[3] > 9900:
            # We must slice data to get the raw bytes, which is slow, or re-pack.
            pass

if __name__ == "__main__":
    main()
