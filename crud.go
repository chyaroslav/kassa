package main

import (
	"log"

	_ "github.com/godror/godror"
)

const (
	tOrder = "НАКЛАДНЫЕ_ККМ_ЧЗ"   //вью с накладными
	tPos   = "ПОЗИЦИИ_ТМЦ_ККМ_ЧЗ" //вью с позициями
)

type O struct {
	OrderId   string `db:"ORDERID" form:"orderid" json:"orderid"`
	OrderNum  string `db:"ORDERNUM" form:"ordernum" json:"ordernum"`
	Client    string `db:"CLIENT" form:"client" json:"client"`
	OrderSum  string `db:"ORDERSUM" form:"ordersum" json:"ordersum"`
	Desc      string `db:"DESCRIPTION" form:"desc" json:"desc"`
	Email     string `db:"EMAIL" form:"email" json:"email"`
	Adv       string `db:"ADV" form:"adv" json:"adv"`
	Positions []*Position
}
type Position struct {
	Barcode   string `db:"BARCODE"`
	Gr        string `db:"GR"`
	Good      string `db:"GOOD"`
	Price     string `db:"PRICE"`
	Cnt       string `db:"CNT"`
	Sum       string `db:"SUM"`
	Pws       string `db:"PWS"`
	Tax       string `db:"TAX"`
	Kiz       string `db:"KIZ"`
	Km_status uint   //статус проверки КМ, заполняется уже перед печатью чека при проверке КМ на сервере
}
type apOrder struct {
	OrderId string `db:"ORDERID"`
	Ptype   int    `db:"PTYPE"`
}

// Выдает позиции накладной по накл_уид, ищет в кэше, если там нет добавляет в кэш из БД
func (k *K) getOrder(ordId string) (*O, error) {

	if len(k.ordCache) > 100 {
		//обнуляем кэш накладных если в нем более 100 элементов
		k.ordCache = make(map[string]*O)
	}
	//Если в кеше есть накладная отдаем позиции
	if o, ok := k.ordCache[ordId]; ok {
		log.Println("Successfully get order from cache:", ordId)
		return o, nil
	}
	//Получаем данные накладной
	qSel := `select t2.УИД orderid, t2.СУММА ordersum,
	t2.НОМЕР||' сумма:'||t2.СУММА||' '||substr(t2.ПРИМЕЧАНИЕ,1,50) Description,
	t2.email Email, t2.АВАНС  Adv  
	from ` + tOrder + `t2 where t2.УИД=:1`
	/* type ord struct {
		OrderId  string  `db:"ORDERID"`
		OrderSum float32 `db:"ORDERSUM"`
		Desc     string  `db:"DESCRIPTION"`
		Email    string  `db:"EMAIL"`
	}
	ot := ord{} */
	o := O{}
	err := k.db.Get(&o, qSel, ordId)
	if err != nil {
		return nil, err
	}
	//o := O{OrderId: ot.OrderId, OrderSum: ot.OrderSum, Desc: ot.Desc, Email: ot.Email, Positions: nil}
	k.ordCache[ordId] = &o
	//Получаем позиции накладной
	qSel = `select barcode,
	t.Группа_тмц gr,
	t.Товар good,
	t.цена price,
	t.КОЛ_ВО cnt,
	t.сумма sum,
	t.ЦЕНА_БЕЗ_СКИДКИ pws,
	t.СТАВКА_НАЛОГА tax,
	t.КИЗ kiz,
from` + tPos + `t where t.НАКЛ_УИД=:1`
	ps := []*Position{}
	err = k.db.Select(&ps, qSel, ordId)
	if err != nil {
		return nil, err
	}
	k.ordCache[ordId].Positions = ps
	log.Println("В кэш добавлены позиции накладной:", ordId)
	return &o, nil
}

func (k *K) getOrders(date string) ([]*O, error) {
	qSel := `select t2.УИД orderid, 
	t2.НОМЕР ordernum, 
	t2.КЛ_НАИМЕНОВАНИЕ client, 
	t2.СУММА ordersum,
	t2.НОМЕР||' сумма:'||t2.СУММА||' '||substr(t2.ПРИМЕЧАНИЕ,1,50) Description,
	t2.email Email,  
	t2.АВАНС Adv
	from ` + tOrder + ` t2 where t2.ОРГ_УИД_ЮРЛИЦО=:1 and t2.ДАТА=to_date(:2,'YYYY-MM-DD')`
	orders := []*O{}
	err := k.db.Select(&orders, qSel, k.params.OrgID, date)
	if err != nil {
		log.Println("error in getOrders", err.Error())
		return nil, err
	}
	return orders, nil
}

// Выдаем список накладных для авто-печати
func (k *K) getAPOrders() ([]*apOrder, error) {
	qSel := `select t2.УИД orderid, decode(t2.ТИП_ОПЛАТЫ,'Н',0,'Б',1) pType
	from ` + tOrder + ` t2 where
	t2.уид in (select п.накл_уид from позиции_характеристик_накл п where п.хар_накл_уид=60 and п.значение='1')`
	// запрос для теста.
	/* qSel := `select t2.УИД orderid, decode(t2.ТИП_ОПЛАТЫ,'Н',0,'Б',1) pType
	from накладные_ккм t2 where t2.ОРГ_УИД_ЮРЛИЦО=203452 and t2.ДАТА=to_date('2023-06-21','YYYY-MM-DD')` */
	orders := []*apOrder{}
	err := k.db.Select(&orders, qSel)
	if err != nil {
		log.Println("error in getAPOrders", err.Error())
		return nil, err
	}
	return orders, nil
}

// Помечаем напечатанную накладную
func (k *K) markOrder(ordId string) error {
	pSql := `declare
	begin
	СОХР_ЗНАЧ_ХАР_НАКЛ (:1, '$ПЕЧЧЕК$', TRUNC (SYSDATE), '1',1);
	end;`
	_, err := k.db.Exec(pSql, ordId)
	if err != nil {
		log.Println("Error when mark printed order:", err.Error())
		return err
	}
	//накладная успешно промаркировалась, удаляем ее из кэша
	log.Println("Удаляем из кэша накладную:", ordId)
	delete(k.ordCache, ordId)
	return nil
}
func (k *K) setOperParams() error {
	var n string
	//Устанавливаем ОргУидЮрлицо
	qGetName := "select наименование from организации t where t.уид=:1"
	err := k.db.Get(&n, qGetName, k.params.OrgID)
	if err != nil {
		log.Println("Ошибка определения ОргУидЮрлицо по УИД:", k.params.OrgID, " ", err)
		return err
	}
	k.kkm.OrgName = n
	qGetName = "select наименование from организации t where t.username=upper(:1)"
	err = k.db.Get(&n, qGetName, k.kkm.User)
	if err != nil {
		log.Println("Ошибка определения организации для пользователя:", k.kkm.User, " ", err)
		return err
	}
	k.kkm.OperName = n
	return nil
}
