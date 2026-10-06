package dubs

func Check(id int64) (checked int, clear bool) {
	remaining, digit := id/10, id%10
	clear = digit == 0
	checked = 1
	for remaining%10 == digit && remaining != 0 {
		checked++
		clear = clear || digit == 0
		remaining, digit = remaining/10, remaining%10
	}
	return checked, clear
}
