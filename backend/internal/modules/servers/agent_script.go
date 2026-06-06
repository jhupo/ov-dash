package servers

const agentScript = `#!/bin/sh
set -eu

num() {
  printf '%s' "$1" | awk '{ if ($1 == "") print 0; else print $1 }'
}

mem_total=$(awk '/MemTotal:/ {print $2 * 1024}' /proc/meminfo 2>/dev/null || echo 0)
mem_available=$(awk '/MemAvailable:/ {print $2 * 1024}' /proc/meminfo 2>/dev/null || echo 0)
swap_total=$(awk '/SwapTotal:/ {print $2 * 1024}' /proc/meminfo 2>/dev/null || echo 0)
swap_free=$(awk '/SwapFree:/ {print $2 * 1024}' /proc/meminfo 2>/dev/null || echo 0)
mem_used=$(( ${mem_total%.*} - ${mem_available%.*} ))
swap_used=$(( ${swap_total%.*} - ${swap_free%.*} ))

disk_line=$(df -B1 / 2>/dev/null | awk 'NR==2 {print $3 " " $2}')
disk_used=$(echo "$disk_line" | awk '{print $1+0}')
disk_total=$(echo "$disk_line" | awk '{print $2+0}')

cpu_line_1=$(awk '/^cpu / {print}' /proc/stat)
sleep 1
cpu_line_2=$(awk '/^cpu / {print}' /proc/stat)
cpu_percent=$(awk -v a="$cpu_line_1" -v b="$cpu_line_2" '
BEGIN {
  split(a, x, " "); split(b, y, " ");
  idle1=x[5]+x[6]; idle2=y[5]+y[6];
  total1=0; total2=0;
  for (i=2; i<=NF; i++) { total1+=x[i]; total2+=y[i]; }
  dt=total2-total1; di=idle2-idle1;
  if (dt <= 0) print 0; else printf "%.2f", (dt-di)*100/dt;
}')

rx1=$(awk 'NR>2 {gsub(":", "", $1); if ($1!="lo") rx+=$2} END {print rx+0}' /proc/net/dev)
tx1=$(awk 'NR>2 {gsub(":", "", $1); if ($1!="lo") tx+=$10} END {print tx+0}' /proc/net/dev)
sleep 1
rx2=$(awk 'NR>2 {gsub(":", "", $1); if ($1!="lo") rx+=$2} END {print rx+0}' /proc/net/dev)
tx2=$(awk 'NR>2 {gsub(":", "", $1); if ($1!="lo") tx+=$10} END {print tx+0}' /proc/net/dev)
rx_rate=$((rx2-rx1))
tx_rate=$((tx2-tx1))

load_values=$(cat /proc/loadavg 2>/dev/null || echo "0 0 0")
load1=$(echo "$load_values" | awk '{print $1+0}')
load5=$(echo "$load_values" | awk '{print $2+0}')
load15=$(echo "$load_values" | awk '{print $3+0}')
tcp_connections=$(awk 'NR>1 {count++} END {print count+0}' /proc/net/tcp 2>/dev/null || echo 0)
udp_connections=$(awk 'NR>1 {count++} END {print count+0}' /proc/net/udp 2>/dev/null || echo 0)
process_count=$(find /proc -maxdepth 1 -type d -regex '/proc/[0-9]+' 2>/dev/null | wc -l | awk '{print $1+0}')
uptime_seconds=$(awk '{print int($1)}' /proc/uptime 2>/dev/null || echo 0)
arch=$(uname -m 2>/dev/null || echo "")
virt=$(systemd-detect-virt 2>/dev/null || true)
os_name=$(grep '^PRETTY_NAME=' /etc/os-release 2>/dev/null | cut -d= -f2- | tr -d '"' || echo "")
cpu_model=$(awk -F': ' '/model name|Hardware/ {print $2; exit}' /proc/cpuinfo 2>/dev/null | sed 's/"/\\"/g')
gpu_model=$(command -v lspci >/dev/null 2>&1 && lspci 2>/dev/null | awk -F': ' '/VGA|3D|Display/ {print $2; exit}' | sed 's/"/\\"/g' || true)
region=$(readlink /etc/localtime 2>/dev/null | awk -F'zoneinfo/' '{print $2}' | awk -F/ '{print $NF}' || true)

printf '{'
printf '"cpu_percent":%s,' "$(num "$cpu_percent")"
printf '"memory_used_bytes":%s,' "$(num "$mem_used")"
printf '"memory_total_bytes":%s,' "$(num "$mem_total")"
printf '"swap_used_bytes":%s,' "$(num "$swap_used")"
printf '"swap_total_bytes":%s,' "$(num "$swap_total")"
printf '"disk_used_bytes":%s,' "$(num "$disk_used")"
printf '"disk_total_bytes":%s,' "$(num "$disk_total")"
printf '"network_rx_bytes":%s,' "$(num "$rx2")"
printf '"network_tx_bytes":%s,' "$(num "$tx2")"
printf '"network_rx_rate_bps":%s,' "$(num "$rx_rate")"
printf '"network_tx_rate_bps":%s,' "$(num "$tx_rate")"
printf '"load1":%s,' "$(num "$load1")"
printf '"load5":%s,' "$(num "$load5")"
printf '"load15":%s,' "$(num "$load15")"
printf '"tcp_connections":%s,' "$(num "$tcp_connections")"
printf '"udp_connections":%s,' "$(num "$udp_connections")"
printf '"process_count":%s,' "$(num "$process_count")"
printf '"uptime_seconds":%s,' "$(num "$uptime_seconds")"
printf '"architecture":"%s",' "$arch"
printf '"virtualization":"%s",' "$virt"
printf '"os_name":"%s",' "$(printf '%s' "$os_name" | sed 's/"/\\"/g')"
printf '"cpu_model":"%s",' "$cpu_model"
printf '"gpu_model":"%s",' "$gpu_model"
printf '"region":"%s"' "$(printf '%s' "$region" | sed 's/"/\\"/g')"
printf '}'
`
