package coord

import "math"

// BDtoGCJ02 将 BD09（百度坐标系）坐标转换为 GCJ02（火星坐标系/高德坐标系）。
//
// 因为用户原始定位使用的是百度地图采集的坐标（BD09），而平台后续需要在高德地图上展示，
// 所以需要在入库存时自动完成 BD09 → GCJ02 转换，并将结果写入 gcj02_longitude/gcj02_latitude 字段。
//
// 转换原理：
//
//	BD09 是在 GCJ02 基础上再次加密得到的，公式为 百度加密 → 逆向解密 → GCJ02。
//	这里直接使用标准的逆向算法还原出 GCJ02 坐标。
//
// 参数：
//
//	bdLng: BD09 经度
//	bdLat: BD09 纬度
//
// 返回值：
//
//	gcjLng: GCJ02 经度
//	gcjLat: GCJ02 纬度
func BDtoGCJ02(bdLng, bdLat float64) (gcjLng, gcjLat float64) {
	// x_pi = π × 3000 / 180，这是百度加密使用的固定常数
	const xPi = math.Pi * 3000.0 / 180.0

	// 先减去百度地图固定的 0.0065/0.006 的偏移量
	x := bdLng - 0.0065
	y := bdLat - 0.006

	// 计算极坐标中的距离和角度，并反向应用百度二次扰动
	z := math.Sqrt(x*x+y*y) - 0.00002*math.Sin(y*xPi)
	theta := math.Atan2(y, x) - 0.000003*math.Cos(x*xPi)

	// 将极坐标转回笛卡尔坐标，即得到 GCJ02 经纬度
	gcjLng = z * math.Cos(theta)
	gcjLat = z * math.Sin(theta)
	return
}
