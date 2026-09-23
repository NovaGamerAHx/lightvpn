#!/usr/bin/env python3
"""Peek at a Windows PE's embedded resources (manifest, version info, icon).

Used to verify that `wails build` embedded build/windows/wails.exe.manifest and
build/windows/info.json with the placeholders substituted. No third-party deps.
"""
import re
import struct
import sys


def parse(path):
    d = open(path, 'rb').read()
    out = {'size': len(d), 'path': path}
    assert d[:2] == b'MZ', 'not a PE file (missing MZ)'
    pe = struct.unpack_from('<I', d, 0x3c)[0]
    assert d[pe:pe + 4] == b'PE\0\0', 'not a PE file (missing PE signature)'
    machine, nsec = struct.unpack_from('<HH', d, pe + 4)
    opt = pe + 24
    magic, = struct.unpack_from('<H', d, opt)
    subsystem, = struct.unpack_from('<H', d, opt + 68)
    out['machine'] = 'AMD64' if machine == 0x8664 else hex(machine)
    out['pe32plus'] = magic == 0x20b
    out['subsystem'] = {2: 'WINDOWS_GUI', 3: 'WINDOWS_CUI'}.get(subsystem, str(subsystem))
    out['os_version'] = '%d.%d' % struct.unpack_from('<HH', d, pe + 4 + 0x10 + 4)[:2]

    sect = opt + (0xf0 if out['pe32plus'] else 0xe0)
    secs = []
    for i in range(nsec):
        o = sect + 40 * i
        name = d[o:o + 8].rstrip(b'\0').decode('latin1')
        vsz, va, rawsz, raw = struct.unpack_from('<IIII', d, o + 8)
        secs.append((va, vsz, raw, rawsz))

    def off(rva):
        for va, vsz, raw, rawsz in secs:
            if va <= rva < va + max(vsz, rawsz):
                return raw + (rva - va)
        return None

    rva, size = struct.unpack_from('<II', d, opt + 112 + 8 * 2)
    out['resources'] = []
    if not rva:
        return out
    base = off(rva)

    def name_at(ptr):
        ln, = struct.unpack_from('<H', d, ptr)
        return d[ptr + 2:ptr + 2 + ln * 2].decode('utf-16le', 'replace')

    def level_entries(o, count_named, count_id):
        res = []
        for k in range(count_named + count_id):
            e = o + 16 + 8 * k
            nm, nxt = struct.unpack_from('<II', d, e)
            label = name_at(base + (nm & 0x7fffffff)) if nm & 0x80000000 else nm
            res.append((label, nxt & 0x7fffffff))
        return res

    n, i = struct.unpack_from('<HH', d, base + 12)
    types = dict()
    for label, val in level_entries(base, n, i):
        if isinstance(label, int):
            types[label] = val

    def datas(typeval):
        o = base + types[typeval]
        n2, i2 = struct.unpack_from('<HH', d, o + 12)
        found = []
        for label, val in level_entries(o, n2, i2):
            o2 = base + val
            n3, i3 = struct.unpack_from('<HH', d, o2 + 12)
            for _lab, val2 in level_entries(o2, n3, i3):
                doff, dsize = struct.unpack_from('<II', d, base + val2)
                found.append((label, dsize, d[base + doff:base + doff + dsize]))
        return found

    if 24 in types:  # RT_MANIFEST
        for label, sz, blob in datas(24):
            out['manifest'] = blob.decode('utf-8', 'replace')
            out['manifest_id'] = label
    if 16 in types:  # RT_VERSION
        for label, sz, blob in datas(16):
            txt = blob.decode('utf-16le', 'ignore')
            parts = [p.strip() for p in re.split(r'[^\x20-\x7e]{2,}', txt) if len(p.strip()) > 2]
            out['version'] = parts
            break
    if 3 in types or 14 in types:
        out['icons'] = len(datas(3)) if 3 in types else 0
        out['icon_groups'] = len(datas(14)) if 14 in types else 0
    return out


if __name__ == '__main__':
    for p in sys.argv[1:]:
        r = parse(p)
        print(f"{r['path']}")
        print(f"  {r['size']/1e6:.2f} MB · {r['machine']} · PE32+={r['pe32plus']} · subsystem {r['subsystem']}")
        if 'manifest' in r:
            print(f"  RT_MANIFEST (id {r['manifest_id']}):")
            for line in r['manifest'].strip().splitlines():
                if line.strip():
                    print('    ' + line.strip()[:150])
        if 'version' in r:
            print('  RT_VERSION strings: ' + ' | '.join(r['version'][:14]))
        if 'icons' in r:
            print(f"  RT_ICON entries: {r['icons']}, groups: {r['icon_groups']}")
