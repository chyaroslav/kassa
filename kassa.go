package main

import (
	"errors"
	"kassa/fptr10"
	"log"
	"strconv"
	"strings"
	"time"
)

// Инициализация и открытие ККМ
func (k *K) init_KKM() (*fptr10.IFptr, error) {

	fptr, err := fptr10.NewSafe()
	if err != nil {
		log.Println("--KKM Driver init error:", err)
		return nil, err
	}
	//defer fptr.Destroy()

	log.Println("--KKM driver version is:", fptr.Version())
	if k.params.KMIP == "" {
		log.Println("IP не задан устанавливаем соединение через COM порт:", k.params.KMPort)
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_MODEL, strconv.Itoa(fptr10.LIBFPTR_MODEL_ATOL_AUTO))
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_PORT, strconv.Itoa(fptr10.LIBFPTR_PORT_COM))
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_COM_FILE, k.params.KMPort)
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_BAUDRATE, strconv.Itoa(fptr10.LIBFPTR_PORT_BR_115200))
		fptr.ApplySingleSettings()
	} else {
		log.Println("Устанавливаем соединение через IP:", k.params.KMIP)
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_PORT, strconv.Itoa(fptr10.LIBFPTR_PORT_TCPIP))
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_IPADDRESS, k.params.KMIP)
		fptr.ApplySingleSettings()
	}

	err = fptr.Open()
	if err != nil {
		log.Println("--KKM open error:", err)
		return nil, err
	}
	k.kkm.IsKKMOpened = true
	return fptr, nil
}

// Проверка доступности ККМ. Если не инициализировано, пытается инициализировать. Возвращает строку с серийным номером в случае успеха или ошибку.
func (k *K) CheckKKM() (string, error) {
	log.Println("CheckKKM...")
	if k.fptr == nil {
		log.Println("--KKM is not initiated.")
		log.Println("--Initiating KKM...")
		f, err := k.init_KKM()
		if err != nil {
			log.Println("--Error when init KKM:", err.Error())
			log.Println("CheckKKM finished unsuccessfully")
			return err.Error(), err
		}
		k.fptr = f
		log.Println("--KKM initiated successfully")
	}
	log.Println("--Query KKM serial number...")
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_DATA_TYPE, fptr10.LIBFPTR_DT_SERIAL_NUMBER)
	k.fptr.QueryData()
	serialNumber := k.fptr.GetParamString(fptr10.LIBFPTR_PARAM_SERIAL_NUMBER)
	if serialNumber == "" {
		log.Println("--Unknown error when query serial number, try to init KKM again")
		log.Println("CheckKKM finished unsuccessfully")
		k.fptr = nil
		return "", nil
	}
	log.Println("--KKM serial number is:", serialNumber)
	log.Println("CheckKKM finished successfully!")
	return serialNumber, nil
}

// Проверка закрыты ли документы в ККТ
func (k *K) checkDocStatus() error {
	log.Println("Запускаем проверку не закрытых документов")
	for {
		if k.fptr.CheckDocumentClosed() == nil {
			log.Println("checkdocclosed=nil")
			break
		}
		// Не удалось проверить состояние документа. Вывести пользователю текст ошибки, попросить устранить неполадку и повторить запрос
		log.Println(k.fptr.ErrorDescription())
		continue
	}

	if !k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_DOCUMENT_CLOSED) {
		// Документ не закрылся. Требуется его отменить (если это чек) и сформировать заново
		log.Println("--Имеется не закрытый документ. Пытаемся отменить")
		err := k.fptr.CancelReceipt()
		if err != nil {
			log.Println("--ошибка отмены чека:", err.Error())
			return err
		}

	}

	if !k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_DOCUMENT_PRINTED) {
		// Можно сразу вызвать метод допечатывания документа, он завершится с ошибкой, если это невозможно
		log.Println("--Имеется не напечатанный документ. Пытаемся допечатать..")
		for {
			err := k.fptr.ContinuePrint()
			if err == nil {
				log.Println("Допечатано успешно")
				return nil
			}
			// Если не удалось допечатать документ - показать пользователю ошибку.
			log.Println("Не удалось напечатать документ. Устраните неполадку и повторите(ожидание 5 мин).", k.fptr.ErrorDescription())
			time.Sleep(5 * time.Minute)
			//return err

			continue
		}
	}
	return nil
}
func (k *K) cancelReceipt() error {
	log.Println("Пытаемся отменить текущий чек")
	err := k.fptr.CancelReceipt()
	if err != nil {
		log.Println("--ошибка отмены чека:", err.Error())
		return err

	}
	return nil
}
func (k *K) checkPaper() bool {
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_DATA_TYPE, fptr10.LIBFPTR_DT_STATUS)
	k.fptr.QueryData()
	isPaperPresent := k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_RECEIPT_PAPER_PRESENT)
	return isPaperPresent
}

// Открытие смены
func (k *K) openShift() error {

	//fmt.Println("Opened ", fptr.IsOpened())
	/* fptr.SetParam(fptr10.LIBFPTR_PARAM_DATA_TYPE, fptr10.LIBFPTR_DT_STATUS)
	fptr.QueryData()
	fmt.Println(fptr.GetParamInt(fptr10.LIBFPTR_PARAM_MODEL), "\n", fptr.GetParamString(fptr10.LIBFPTR_PARAM_SERIAL_NUMBER)) */
	//k.fptr.IsOpened()
	k.fptr.SetParam(1021, k.kkm.OperName)
	k.fptr.SetParam(1203, "123456789047")
	log.Println("--Trying to login into KKM. Operator: ", k.kkm.OperName)
	err := k.fptr.OperatorLogin()
	if err != nil {
		log.Println("--Error while Operator login in KKM: ", err)
		return err
	}
	log.Println("--Operator: ", k.kkm.OperName, " successfully logged in")
	log.Println("--Trying to open shift...")
	err = k.fptr.OpenShift()
	if err != nil {
		log.Println("--Error while Open Shift in KKM: ", err)
		//return err
		//Убрали выход и выдачу ошибки так как смена может быть уже открыта
	}
	log.Println("--Shift successfully opened")
	log.Println("--Check unclosed documents...")
	/* chk := k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_DOCUMENT_CLOSED)
	if !chk {
		log.Println("--Имеется не закрытый документ, отменяем")
		err = k.fptr.CancelReceipt()
		if err != nil {
			log.Println("Ошибка отмены чека:", err.Error())
			return err
		}
	} */
	k.checkDocStatus()
	/* err = k.fptr.CheckDocumentClosed()
	if err != nil {
		log.Println("--Error when check unclosed dockuments: ", err)
		log.Println("Пробуем отменить открытый чек")
		err = k.fptr.CancelReceipt()
		if err != nil {
			log.Println("Ошибка отмены чека:", err.Error())
			return err
		}
	} */
	//k.fptr.SetParam(fptr10.LIBFPTR_PARAM_REPORT_TYPE, fptr10.LIBFPTR_RT_CLOSE_SHIFT)
	//k.fptr.Report()
	log.Println("--Opening shift finished")
	k.kkm.IsShiftOpened = true
	return nil
}
func (k *K) closeShift() error {
	log.Println("Закрываем смену...")
	k.checkDocStatus()
	k.fptr.SetParam(1021, k.kkm.OperName)
	k.fptr.SetParam(1203, "123456789047")
	/* err := k.fptr.CheckDocumentClosed()
	if err != nil {
		log.Println("ошибка при проверке открытых документов при закрытии смены: ", err)
		return err
	} */

	err := k.fptr.OperatorLogin()
	if err != nil {
		log.Println("Error while Operator login in KKM: ", err)
		return err
	}
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_REPORT_TYPE, fptr10.LIBFPTR_RT_CLOSE_SHIFT)
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_RECEIPT_ELECTRONICALLY, true)
	err = k.fptr.Report()
	if err != nil {
		log.Println("Error while print closing report: ", err)
		return err
	}

	k.kkm.IsShiftOpened = false
	log.Println("Смена закрыта успешно")
	return nil
}

// переоткрытие смены
func (k *K) reopenShift() error {
	log.Println("Закрываем смену..")
	err := k.closeShift()
	if err != nil {
		log.Println("Закрытие смены завершилось с ошибкой..")
		return err
	}
	log.Println("Открываем смену..")
	err = k.openShift()
	if err != nil {
		log.Println("Открытие смены завершилось ошибкой..")
		return err
	}
	return nil
}
func (k *K) setTax(tax string) {
	switch tax {
	case "3":
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TAX_TYPE, fptr10.LIBFPTR_TAX_VAT20)
	case "2":
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TAX_TYPE, fptr10.LIBFPTR_TAX_VAT10)
	case "1":
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TAX_TYPE, fptr10.LIBFPTR_TAX_NO)
	case "5":
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TAX_TYPE, fptr10.LIBFPTR_TAX_VAT5)
	case "7":
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TAX_TYPE, fptr10.LIBFPTR_TAX_VAT7)
	}
}

/* func (k *K) setCustomParams() {
	switch k.params.CompanyName {
	case "derufa":
		k.fptr.SetParam(2108, 0)
		log.Println("derufa param 2108 set to 0")
	}
} */
/* func (k *K) setRCustomParams(o *O) {
	switch k.params.CompanyName {
	case "derufa":
		a, _ := strconv.Atoi(o.Adv)
		if a == 1 {
			k.fptr.SetParam(1214, 3)
			log.Println("derufa param 1214 set to 3")
		}
	}
} */
func strToFloat(s string) (float64, error) {
	//Удаляем пробелы
	s1 := strings.ReplaceAll(s, " ", "")
	//Заменяем зпт на точку
	s1 = strings.Replace(s1, ",", ".", 1)
	f, err := strconv.ParseFloat(s1, 64)
	if err != nil {
		//log.Println(err.Error())
		return 0, err
	}
	return f, nil
}

// проверка КМ и заполнение статуса проверки
func (k *K) checkKM(o *O) error {

	k.fptr.UpdateFnmKeys()
	k.fptr.ClearMarkingCodeValidationResult()
	log.Println("Результат обновления ключей:", k.fptr.GetParamString(fptr10.LIBFPTR_PARAM_MARKING_SERVER_ERROR_DESCRIPTION))
	err := k.fptr.PingMarkingServer()
	log.Println("пинг сервера ИСМ:", err)
	// Ожидание результатов проверки связи с сервером ИСМ
	for {
		k.fptr.GetMarkingServerStatus()

		if k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_CHECK_MARKING_SERVER_READY) {
			log.Println("Сервер ИСМ доступен")
			break
		}
	}
	errorCode := k.fptr.GetParamInt(fptr10.LIBFPTR_PARAM_MARKING_SERVER_ERROR_CODE)
	errorDescription := k.fptr.GetParamString(fptr10.LIBFPTR_PARAM_MARKING_SERVER_ERROR_DESCRIPTION)
	log.Println("отклик сервера маркировки:", k.fptr.GetParamInt(fptr10.LIBFPTR_PARAM_MARKING_SERVER_RESPONSE_TIME))
	if errorCode != 0 {
		log.Println("ism error code:", errorCode, " ", errorDescription)
		return errors.New(errorDescription)
	}
	// Запускаем проверку КМ
	for _, pos := range o.Positions {
		//runes := []rune(pos.Kiz)
		log.Printf("Начинаем проверку КМ\n")
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_CODE_TYPE, fptr10.LIBFPTR_MCT12_AUTO)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_CODE, pos.Kiz)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_CODE_STATUS, fptr10.LIBFPTR_MES_PIECE_SOLD)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_WAIT_FOR_VALIDATION_RESULT, true)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TIMEOUT, 10000)
		//k.fptr.SetParam(fptr10.LIBFPTR_PARAM_QUANTITY, 1.000)
		//k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MEASUREMENT_UNIT, fptr10.LIBFPTR_IU_PIECE)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_PROCESSING_MODE, 0)
		//k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_FRACTIONAL_QUANTITY, "1/2")
		k.fptr.BeginMarkingCodeValidation()
		//time.Sleep(5 * time.Second)
		// Дожидаемся окончания проверки и запоминаем результат
		/* for {
			k.fptr.GetMarkingCodeValidationStatus()
			if k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_MARKING_CODE_VALIDATION_READY) {
				break
			}
		} */
		log.Println("Готовность проверки маркировки:", k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_MARKING_CODE_VALIDATION_READY))
		pos.Km_status = k.fptr.GetParamInt(fptr10.LIBFPTR_PARAM_MARKING_CODE_ONLINE_VALIDATION_RESULT)
		// Подтверждаем реализацию товара с указанным КМ
		log.Println("проверка маркировки товара:", pos.Good, " статус:", pos.Km_status)
		k.fptr.AcceptMarkingCode()
		k.fptr.ClearMarkingCodeValidationResult()
	}
	log.Println("Проверка КМ завершена")
	return nil
}

// Печать позиций накладной на ККМ, параметры: накл_уид; тип оплаты 0-нал, 1-безнал; электронный чек true\false
func (k *K) printOrderPos(ordId string, pType int, pEl bool) error {
	var ordSum float64

	log.Println("--Starting print order")
	o, err := k.getOrder(ordId)
	if err != nil {
		log.Println("--ошибка получения накладной:", ordId, " -", err)
		return err
	}

	//Прописываем в чек эл.почту если она заполнена
	if o.Email != "" {
		k.fptr.SetParam(1008, o.Email)
		//k.fptr.UtilFormTlv()
		//clientInfo := k.fptr.GetParamByteArray(fptr10.LIBFPTR_PARAM_TAG_VALUE)
		//k.fptr.SetParam(1256, clientInfo)
	}
	log.Println("sum:", o.OrderSum)
	ordSum, err = strToFloat(o.OrderSum)
	if err != nil {
		log.Println("Ошибка конвертации суммы накладной", err.Error())
		return err
	}
	//Проверка кода маркировки с заполнением статусов проверки для добавления в чек
	km_checked := true
	err = k.checkKM(o)
	if err != nil {
		log.Println("--ошибка проверки кодов маркировки ", err)
		km_checked = false
		return err //временный выход чтобы не печатать без КМ
	}
	if ordSum < 0 {
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_RECEIPT_TYPE, fptr10.LIBFPTR_RT_SELL_RETURN)
		//ordSum = ordSum * (-1)
	} else {
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_RECEIPT_TYPE, fptr10.LIBFPTR_RT_SELL)
		//log.Println("install receipt_type")
	}

	//Устанавливаем электронный чек (без печати) если задано
	if pEl {
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_RECEIPT_ELECTRONICALLY, true)
		log.Println("Печатаем без бумаги..")
	}
	err = k.fptr.OpenReceipt()
	if err != nil {
		log.Println("--ошибка открытия чека: ", err)
		//Если ошибка в превышении смены в 24 часа, пытаемся переоткрыть смену.
		if k.fptr.ErrorCode() == 68 {
			log.Println("Смена превысила 24 часа, пытаемся переоткрыть..")
			err = k.reopenShift()
			if err != nil {
				log.Println("Переоткрытие смены завершилось не удачно..")
				return err
			}
			err = k.fptr.OpenReceipt()
			if err != nil {
				log.Println("--ошибка открытия чека: ", err)
				return err
			}
		} else {
			return err
		}

	}
	//Считаем сумму как сумму позиций
	ordSum = 0
	for _, pos := range o.Positions {
		price, err := strToFloat(pos.Price)
		if err != nil {
			log.Println("--ошибка конвертации цены в float: ", err)
			return err
		}
		cnt, err := strToFloat(pos.Cnt)
		if err != nil {
			log.Println("--ошибка конвертации количество в float: ", err)
			return err
		}

		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_COMMODITY_NAME, pos.Good)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_PRICE, price)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_QUANTITY, cnt)
		k.setTax(pos.Tax)
		k.fptr.SetParam(2108, 0) //устанавливаем measurementUnit=0 - piece - штуки, единицы
		//k.setCustomParams()
		//Код который по параметру накладной устанавливает параметр позиции в зависимости от компании
		//k.setRCustomParams(o) -- пока не используется
		if km_checked {
			//параметры для маркировки:
			log.Println("Устанавливаем параметры маркировки: КМ:", pos.Kiz, "статус проверки:", pos.Km_status)
			//k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_FRACTIONAL_QUANTITY, "1/2")
			k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_CODE, pos.Kiz)
			k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_CODE_STATUS, fptr10.LIBFPTR_MES_PIECE_SOLD)
			k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_CODE_ONLINE_VALIDATION_RESULT, pos.Km_status)
			k.fptr.SetParam(fptr10.LIBFPTR_PARAM_MARKING_PROCESSING_MODE, 0)
		}
		//устанавливаем тип платежа Аванс если это указано в накладной (поле Adv=1) пока используется только в деруфе
		adv, _ := strconv.Atoi(o.Adv)
		if adv == 1 {

			k.fptr.SetParam(1214, 3)
		}
		err = k.fptr.Registration()
		if err != nil {
			log.Println("--ошибка регистрации позиции: ", err)
			return err
		}
		s, err := strToFloat(pos.Sum)
		if err != nil {
			log.Println("--ошибка конвертации суммы позиции в float: ", err)
			return err
		}
		ordSum = ordSum + s
	}
	//pType = 0 = fptr10.LIBFPTR_PT_cache - наличные, pType = 1 = fptr10.LIBFPTR_PT_ELECTRONICALLY - безнал
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_PAYMENT_TYPE, pType)
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_PAYMENT_SUM, ordSum)

	err = k.fptr.Payment()
	if err != nil {
		log.Println("--ошибка осуществления платежа: ", err)
		return err
	}
	// Перед закрытием проверяем, что все КМ отправились (на случай, если были проверки КМ без ожидания результата
	for {
		k.fptr.CheckMarkingCodeValidationsReady()
		if k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_MARKING_CODE_VALIDATION_READY) {
			break
		}
	}
	err = k.fptr.CloseReceipt()
	if err != nil {
		log.Println("--ошибка закрытия чека: ", err)
		return err
	}
	/* if k.checkPaper() {
		k.sendLogMsg("ВНИМАНИЕ! Заканчивается чековая лента")
	} */
	return nil
}
