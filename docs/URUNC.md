# urunc: The Runtime Model Behind This Platform

A grounded reference for *why* this project is built the way it is. Every claim
here is traceable to the official urunc documentation; where a capability is
**not** available, it is flagged so the platform never over-promises.

Primary sources:
- Overview & support matrix — <https://urunc.io/>
- Design & lifecycle — <https://urunc.io/design/>
- Packaging & annotations — <https://urunc.io/package/>
- Hypervisor support — <https://urunc.io/hypervisor-support/>
- Configuration — <https://urunc.io/configuration/>
- Seccomp — <https://urunc.io/design/seccomp/>
- Source — <https://github.com/nubificus/urunc>
- Inspiration — <https://nubificus.co.uk/blog/urunc_agent/>

---

## 1. What urunc is

urunc is a **container runtime that boots unikernels and microVMs instead of
ordinary Linux containers**. It implements the containerd shim interface
(`containerd-shim-urunc-v2`) and the OCI runtime contract, so `nerdctl` and
Kubernetes drive it exactly like `runc` — `--runtime io.containerd.urunc.v2`.
The difference is what runs inside: a real VM with its own kernel, isolated by
the hypervisor boundary, not a namespaced process sharing the host kernel.

That hardware-backed boundary is the entire reason this platform exists: a
compromised tool is trapped inside a VM, not merely inside a namespace.

## 2. Unikernel × Monitor × Storage support matrix

From <https://urunc.io/>. This drives which monitors a given tool image can use.

| Unikernel | Monitors | Arch | Storage |
|-----------|----------|------|---------|
| Rumprun | Solo5-hvt, Solo5-spt | x86, aarch64 | Block/Devmapper |
| Unikraft | QEMU, Firecracker | x86 | Initrd, 9pfs |
| MirageOS | QEMU, Solo5-hvt, Solo5-spt | x86, aarch64 | Block/Devmapper |
| **Linux** | **QEMU, Firecracker** | x86, aarch64 | Initrd, Block, 9pfs, Virtiofs |
| Hermit | QEMU | x86 | Initrd |

**Consequence for this platform:** our practical tools use **Linux** images, so
they can only run on **QEMU or Firecracker** — *never Solo5*. That is why the
policy schema restricts `monitor` to `qemu`/`firecracker` and why the Solo5
monitors appear only in the documented matrix, not as Linux tool options. The
`nginx_showcase` tool uses a prebuilt **Unikraft** image to demonstrate a true
unikernel honestly.

## 3. Monitor trade-offs

From <https://urunc.io/hypervisor-support/> and the Firecracker/Unikraft papers.

| Monitor | VM boot | Memory overhead | TCB | Notes |
|---------|---------|-----------------|-----|-------|
| Solo5-hvt | ~3 ms | minimal | smallest | Rumprun/MirageOS only |
| Firecracker | ~3 ms | < 5 MiB | small (Rust) | Linux/Unikraft; Linux boot can be version-sensitive |
| QEMU | ~40 ms | 100s MiB | largest | broadest compatibility, most tested |

This platform **defaults to QEMU** for reliability and lets a tool opt into
Firecracker via policy when fast cold-start matters — but only after
`make verify-monitors` confirms Firecracker can boot a Linux image in the host.

## 4. OCI annotations (`com.urunc.unikernel.*`)

From <https://urunc.io/package/>. urunc reads these to know what to boot.

| Annotation | Meaning |
|------------|---------|
| `unikernelType` | framework: `linux`, `unikraft`, `rumprun`, `mirage` |
| `hypervisor` | monitor: `qemu`, `firecracker`, `hvt`, `spt` |
| `binary` | path to the unikernel/init binary in the rootfs |
| `cmdline` | command line passed to the guest |
| `mountRootfs` | mount the container rootfs inside the guest |
| `initrd` / `block` / `blkMntPoint` | storage source selection |

These are baked into the image at build time by **bunny** (our `build/Containerfile`
sets them as `LABEL`s). At run time, `nerdctl --annotation k=v` can override
them — which is exactly how this platform switches a Linux tool's monitor
without rebuilding the image. We deliberately do **not** override annotations on
prebuilt unikernel images, whose values are baked correctly by their publisher.

## 5. Container lifecycle (design)

From <https://urunc.io/design/>: containerd unpacks the image to a snapshotter →
urunc parses rootfs + annotations → creates a network namespace → sets up a TAP
device and attaches storage → selects the monitor → launches the VM and manages
it as the container process. `--rm` tears the whole VM down on exit. This is why
"one tool call → one microVM → destroyed on return" is literally true here.

## 6. Networking

From <https://urunc.io/design/>: urunc creates a TAP device per VM inside the
network namespace and integrates with **standard CNI plugins**. Our CNI config
(`configs/cni/10-urunc-bridge.conf`) is a bridge `urunc0` on `10.99.0.0/24`.

`--network=none` attaches no NIC at all (true air-gap). `--network=bridge` gives
the VM an address on the bridge with **full, unfiltered egress** to anywhere the
host can route. There is **no per-destination egress filter** exposed by
nerdctl/urunc today — see the honesty note in §8.

## 7. Seccomp — what it actually protects

From <https://urunc.io/design/seccomp/>: seccomp filtering in urunc applies to
the **host-side monitor process** (e.g. QEMU), shrinking the syscalls the VMM
can make against the host. Firecracker enables it by default; for QEMU urunc
activates sandbox filters; Solo5-spt applies a 7-syscall whitelist natively.

**Critical nuance this platform documents honestly:** a custom seccomp profile
passed via `--security-opt seccomp=` filters the *monitor*, **not** syscalls
*inside* the guest. The guest has its own kernel and is already isolated by the
VM boundary. So `configs/seccomp/strict.json` is *defence-in-depth for the host*
(deny module-loading, mount, ptrace, eBPF, etc. that a VMM never needs) — it does
not, and is not claimed to, restrict what code does inside the VM.

## 8. Honest capability ledger

| Dimension | Enforced? | Mechanism |
|-----------|-----------|-----------|
| Network `none` | ✅ | `--network=none`, no NIC in the VM |
| Network `bridge` | ✅ | CNI bridge `urunc-net` |
| **Egress allowlist** | ❌ declared only | no per-destination filter in nerdctl/urunc; bridge = full egress. Real path: a chained CNI firewall plugin (CIDR-only) or an egress proxy. The platform logs a warning and marks `egress_enforced:false`. |
| Monitor (QEMU/Firecracker) | ✅* | `--annotation com.urunc.unikernel.hypervisor` (*Firecracker-on-Linux must be verified per host) |
| Seccomp (monitor) | ✅ | `--security-opt seccomp=` — hardens the monitor, not the guest (§7) |
| Read-only rootfs | ✅ | `--read-only` |
| Memory / CPU | ✅ | `-m` / `--cpus` |
| Per-tool timeout | ✅ | `context.WithTimeout` in the manager |
| Filesystem mounts | ✅ | `-v host:container[:ro]` |
| Boot telemetry | ✅ (serialized) | urunc `[timestamps]` log; attribution serialized because all VMs share one log |

## 9. Things urunc does NOT provide (so we don't claim them)

Verified absent from the docs at time of writing:
- VM **snapshot/restore** (a Firecracker feature, not surfaced by urunc)
- Firecracker **jailer** integration
- **Custom seccomp policy** for the *guest* (profiles target the monitor)
- Per-container **egress domain filtering**
- **Live migration**

Building on top of urunc means inheriting these boundaries. The platform's value
is the *policy + orchestration layer* above urunc, expressed honestly.
