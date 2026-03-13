package battery_info

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/distatus/battery"
	"github.com/fatih/color"
)

const sampleWindow = 5

var (
	powerHistory     []float64
	powerHistoryMu   sync.Mutex
	lastBatteryState string
)

func getAveragePowerRate(bat *battery.Battery) float64 {
	powerHistoryMu.Lock()
	defer powerHistoryMu.Unlock()

	currentState := bat.State.String()
	if currentState != lastBatteryState && (currentState == "Charging" || currentState == "Discharging") {
		powerHistory = nil
		lastBatteryState = currentState
	}

	rate := bat.ChargeRate / 1000
	powerHistory = append(powerHistory, rate)
	if len(powerHistory) > sampleWindow {
		powerHistory = powerHistory[1:]
	}

	var sum float64
	for _, r := range powerHistory {
		sum += r
	}
	return sum / float64(len(powerHistory))
}

// 用于封装电池信息的结构体
type BatteryInfo struct {
	State      string  // 带颜色的状态名，如绿色的"已充满"
	CurrentPct string  // 带颜色的当前电量百分比，如绿色的"85%"
	FullWh     float64 // 以 Wh 表示的当前最大容量
	DesignWh   float64 // 以 Wh 表示的设计容量
	PowerRate  string  // 以瓦特为单位，可认为是瞬时功率
	HealthPct  string  // 带颜色的健康度，如绿色的"95.1%"
}

// 获取并格式化电池信息
func GetBatteryInfo(bat *battery.Battery) BatteryInfo {
	health := (bat.Full / bat.Design) * 100         // 健康度计算公式
	batteryNumber := (bat.Current / bat.Full) * 100 // 电量值，使用计算的原因是，battery 库无法直接获取电量值
	var currentPct string
	if batteryNumber > 80 {
		currentPct = color.GreenString("%.1f%%", batteryNumber)
	} else if batteryNumber > 30 {
		currentPct = color.YellowString("%.1f%%", batteryNumber)
	} else {
		currentPct = color.RedString("%.1f%%", batteryNumber)
	}

	var healthPct string
	if health > 90 {
		healthPct = color.GreenString("%.1f%%", health)
	} else if health > 80 {
		healthPct = color.YellowString("%.1f%%", health)
	} else {
		healthPct = color.RedString("%.1f%%", health)
	}

	var stateColorString string
	var powerRateColorString string
	effectiveRate := getAveragePowerRate(bat)
	switch bat.State.String() {
	case "Full":
		stateColorString = color.GreenString("已充满")
	case "Discharging":
		stateColorString = color.YellowString("放电中")
		powerRateColorString = color.YellowString("%.1fW", effectiveRate)
	case "Charging":
		stateColorString = color.GreenString("充电中")
		powerRateColorString = color.GreenString("%.1fW", effectiveRate)
	case "Idle":
		stateColorString = color.BlackString("未使用")
	default:
		stateColorString = bat.State.String()
	}

	return BatteryInfo{
		State:      stateColorString,
		CurrentPct: currentPct,
		FullWh:     bat.Full / 1000,
		DesignWh:   bat.Design / 1000,
		PowerRate:  powerRateColorString,
		HealthPct:  healthPct,
	}
}

// 打印电池信息
func PrintBatteryInfo(info BatteryInfo, index int) {
	if index > 0 {
		fmt.Printf("电池 %d:\n", index)
	}
	fmt.Printf("当前电池状态\t%s\n", info.State)
	fmt.Printf("电池的净功率\t%s\n", info.PowerRate)
	fmt.Printf("当前电池电量\t%s\n", info.CurrentPct)
	fmt.Printf("实际最大电量\t%.1f Wh\n", info.FullWh)
	fmt.Printf("设计最大电量\t%.1f Wh\n", info.DesignWh)
	fmt.Printf("电池的健康度\t%s\n", info.HealthPct)
}

// 格式化剩余时间为可读字符串
func formatRemainingTime(duration time.Duration) string {
	totalMinutes := int(duration.Minutes())
	hours := totalMinutes / 60
	minutes := totalMinutes % 60

	if totalMinutes < 60 {
		return fmt.Sprintf("（%d 分钟后）", totalMinutes)
	}
	return fmt.Sprintf("（%d 小时 %d 分钟后）", hours, minutes)
}

// 打印监控信息
func printMonitorInfo(bat *battery.Battery) {
	now := time.Now()
	nowStr := color.CyanString(now.Format("15:04"))
	info := GetBatteryInfo(bat)
	if bat.State.String() == "Charging" {
		powerRate := getAveragePowerRate(bat)
		remainingmwH := bat.Full - bat.Current
		if powerRate > 0.01 {
			remainingHours := float64(remainingmwH) / (powerRate * 1000)
			remainingTime := time.Duration(remainingHours * float64(time.Hour))
			predictTime := now.Add(remainingTime)
			timeHint := formatRemainingTime(remainingTime)
			fmt.Printf("%s\t电量：%s\t功率：%s\t预计于 %s %s %s\n", nowStr, info.CurrentPct, info.PowerRate, color.BlueString(predictTime.Format("1 月 2 日 15:04")), color.GreenString("充满"), color.HiMagentaString(timeHint))
		} else {
			fmt.Printf("%s\t电量：%s\t%s\n", nowStr, info.CurrentPct, color.GreenString("处于直接电源供电状态，或电池的净功率非常低"))
		}

	} else if bat.State.String() == "Discharging" {
		powerRate := getAveragePowerRate(bat)
		remainingmwH := bat.Current - bat.Full*0.2
		if powerRate > 0.01 {
			remainingHours := float64(remainingmwH) / (powerRate * 1000)
			remainingTime := time.Duration(remainingHours * float64(time.Hour))
			predictTime := now.Add(remainingTime)
			timeHint := formatRemainingTime(remainingTime)
			fmt.Printf("%s\t电量：%s\t功率：%s\t预计于 %s %s %s\n", nowStr, info.CurrentPct, info.PowerRate, color.BlueString(predictTime.Format("1 月 2 日 15:04")), color.RedString("消耗至 20%"), color.HiMagentaString(timeHint))
		} else {
			fmt.Printf("%s\t电量：%s\t%s\n", nowStr, info.CurrentPct, color.GreenString("处于直接电源供电状态，或电池的净功率非常低"))
		}
	} else {
		fmt.Printf("%s\t电量：%s\t%s\n", nowStr, info.CurrentPct, color.GreenString("处于直接电源供电状态，或电池的净功率非常低"))
	}
}

// 主函数，处理所有电池信息获取和打印
func Run() {
	batteries, err := battery.GetAll()
	if err != nil {
		color.Red("无法获取电池信息")
		return
	}
	for i, bat := range batteries {
		info := GetBatteryInfo(bat)
		PrintBatteryInfo(info, i)
	}

	fmt.Printf("===========\n")
	fmt.Println("进入监测模式，按 Ctrl+C 退出")
	fmt.Printf("说明：输出的功率为%s，是指电池自身的实际功率，而非电脑的功率\n", color.CyanString("净功率"))
	fmt.Printf("功率数字为%s表示放电中，功率颜色为%s表示充电中\n", color.YellowString("黄色"), color.GreenString("绿色"))
	fmt.Printf("电量颜色为%s表示电量充足，%s表示电量中等，%s表示电量低\n", color.GreenString("绿色"), color.YellowString("黄色"), color.RedString("红色"))
	fmt.Printf("健康度颜色为%s表示健康，%s表示一般，%s表示较差\n", color.GreenString("绿色"), color.YellowString("黄色"), color.RedString("红色"))
	fmt.Printf("仅在电池状态为%s或%s时，才会显示功率和预计充满或耗尽时间\n", color.GreenString("充电中"), color.YellowString("放电中"))
	fmt.Printf("===========\n")

	// 监控模式：每分钟输出简短信息
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// 进入监控时立即输出一条信息
	batteries, err = battery.GetAll()
	if err == nil {
		for _, bat := range batteries {
			printMonitorInfo(bat)
		}
	}

	// 信号处理：监听 Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	for {
		select {
		case <-ticker.C:
			batteries, err := battery.GetAll()
			if err != nil {
				continue
			}
			for _, bat := range batteries {
				printMonitorInfo(bat)
			}
		case <-sigChan:
			fmt.Println("监测结束")
			return
		}
	}
}
