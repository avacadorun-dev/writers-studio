import { useState } from "react";

import {
  interfaceLanguage,
  interfaceLanguages,
  isInterfaceLanguage,
  saveInterfaceLanguage,
  t,
} from "~shared/i18n";

import { Button } from "./button";
import { Field, Select } from "./field";

const names = { en: "English", ru: "Русский", "zh-Hans": "简体中文" } as const;

export function InterfaceLanguagePicker() {
  const [language, setLanguage] = useState(interfaceLanguage);
  const [error, setError] = useState<string>();

  const apply = () => {
    try {
      saveInterfaceLanguage(language);
    } catch {
      setError(t("Не удалось сохранить язык интерфейса. Проверьте настройки хранилища браузера."));
      return;
    }
    window.location.reload();
  };

  return (
    <div className="grid items-start gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
      <Field
        label={t("Язык интерфейса")}
        hint={t("Язык применяется после перезагрузки страницы.")}
        error={error}
      >
        <Select
          value={language}
          onChange={(event) => {
            const selected = event.target.value;
            if (isInterfaceLanguage(selected)) setLanguage(selected);
            setError(undefined);
          }}
        >
          {interfaceLanguages.map((option) => (
            <option key={option} value={option} lang={option}>
              {names[option]}
            </option>
          ))}
        </Select>
      </Field>
      <Button
        className="sm:mt-7"
        type="button"
        disabled={language === interfaceLanguage}
        onClick={apply}
      >
        {t("Применить язык")}
      </Button>
    </div>
  );
}
