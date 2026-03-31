package queue

/*队列通知接口*/
type Queue interface {
	/*接收一个值*/
	Push(val any)
	/*取出一个值*/
	Pop() any
	/*是否为空*/
	Empty() bool

	Destroy()
}
