package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	Red    = "\033[31m"
	Green  = "\033[32m"
	Blue   = "\033[34m"
	Purple = "\033[35m"
	Reset  = "\033[0m"
)

type SearchResponse struct {
	Data struct {
		ASNs []struct {
			ASN  int    `json:"asn"`
			Name string `json:"name"`
		} `json:"asns"`
	} `json:"data"`
}

type PrefixResponse struct {
	Data struct {
		IPv4Prefixes []struct {
			Prefix string `json:"prefix"`
		} `json:"ipv4_prefixes"`
	} `json:"data"`
}

func getJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func getASNs(orgName string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("https://api.bgpview.io/search?query_term=%s", orgName)
	var result SearchResponse
	if err := getJSON(url, &result); err != nil {
		return nil, err
	}

	asns := make([]map[string]interface{}, len(result.Data.ASNs))
	for i, a := range result.Data.ASNs {
		asns[i] = map[string]interface{}{
			"asn":  a.ASN,
			"name": a.Name,
		}
	}
	return asns, nil
}

func getIPRanges(asn int) ([]string, error) {
	url := fmt.Sprintf("https://api.bgpview.io/asn/%d/prefixes", asn)
	var result PrefixResponse
	if err := getJSON(url, &result); err != nil {
		return nil, err
	}

	ranges := []string{}
	for _, p := range result.Data.IPv4Prefixes {
		ranges = append(ranges, p.Prefix)
	}
	return ranges, nil
}

func reverseLookup(ip string) []string {
	names, err := net.LookupAddr(ip)
	if err != nil {
		return []string{}
	}
	return names
}

func ipsInCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		ipCopy := make(net.IP, len(ip))
		copy(ipCopy, ip)
		ips = append(ips, ipCopy.String())
	}

	if len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}
	return ips, nil
}

func incIP(ip net.IP) {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return
	}
	binary.BigEndian.PutUint32(ipv4, binary.BigEndian.Uint32(ipv4)+1)
}

func printBanner() {
	fmt.Println(Purple + `   ______________   _
                   / )
    \______       (_/ \
         \_ ) __      /        
             (___\ \  
            (____/  \
             (___/   \
s-v            ( ____/` + Reset)
	fmt.Println()
	fmt.Println(Green + "[+] github.com/unvalidor")
	fmt.Println("[+] linkedin.com/in/unvalidor")
	fmt.Println("[+] Usage : go run asn-lookup.go" + Reset)
	fmt.Println()
}

func main() {
	printBanner()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print(Blue + "Enter domain or company name: " + Reset)
	orgName, _ := reader.ReadString('\n')
	orgName = strings.TrimSpace(orgName)

	if orgName == "" {
		fmt.Println(Red + "Error: Please enter a valid organization name." + Reset)
		os.Exit(1)
	}

	asns, err := getASNs(orgName)
	if err != nil {
		fmt.Println(Red+"Error fetching ASNs:", err, Reset)
		os.Exit(1)
	}

	if len(asns) == 0 {
		fmt.Printf(Red+"No ASN found for %s\n"+Reset, orgName)
		os.Exit(0)
	}

	fmt.Printf(Green+"\n[+] Found ASNs for %s\n"+Reset, orgName)
	for i, asn := range asns {
		fmt.Printf(Blue+"%d."+Reset+" AS%d - %s\n", i+1, int(asn["asn"].(int)), asn["name"].(string))
	}

	fmt.Print(Purple + "\nSelect ASN number: " + Reset)
	var choiceStr string
	fmt.Scanln(&choiceStr)
	choice, err := strconv.Atoi(choiceStr)
	if err != nil || choice < 1 || choice > len(asns) {
		fmt.Println(Red + "Invalid selection." + Reset)
		os.Exit(1)
	}

	selectedASN := int(asns[choice-1]["asn"].(int))
	ipRanges, err := getIPRanges(selectedASN)
	if err != nil {
		fmt.Println(Red+"Error fetching IP ranges:", err, Reset)
		os.Exit(1)
	}

	fmt.Printf(Green+"\n[+] IP ranges for ASN %d:\n"+Reset, selectedASN)
	for _, ip := range ipRanges {
		fmt.Println(ip)
	}

	fmt.Printf(Purple+"\n[~] Starting reverse DNS lookups for all IPs in found ranges...\n"+Reset)
	time.Sleep(1 * time.Second)

	for _, prefix := range ipRanges {
		allIPs, err := ipsInCIDR(prefix)
		if err != nil {
			fmt.Println(Red+"[!] Failed to parse CIDR:", prefix, err, Reset)
			continue
		}

		fmt.Printf(Green+"\n[+] Scanning %d IPs in %s\n"+Reset, len(allIPs), prefix)

		for _, ip := range allIPs {
			domains := reverseLookup(ip)
			if len(domains) > 0 {
				fmt.Printf(Blue+"[+] %s -> %s\n"+Reset, ip, strings.Join(domains, ", "))
			}
			time.Sleep(100 * time.Millisecond) 
		}
	}
}
